package recovery

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Scheduler sweeps for expired leases on a fixed interval and recovers affected jobs.
// Embedded in forge-api as a goroutine (Architecture OD-003).
type Scheduler struct {
	pool          *pgxpool.Pool
	sweepInterval time.Duration
	log           zerolog.Logger
}

func NewScheduler(pool *pgxpool.Pool, sweepInterval time.Duration, log zerolog.Logger) *Scheduler {
	return &Scheduler{pool: pool, sweepInterval: sweepInterval, log: log}
}

// Run sweeps on sweepInterval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	s.log.Info().Dur("interval", s.sweepInterval).Msg("recovery scheduler started")
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
				s.log.Error().Err(err).Msg("recovery sweep error")
			}
		case <-ctx.Done():
			s.log.Info().Msg("recovery scheduler stopped")
			return
		}
	}
}

// Sweep finds all CLAIMED/RUNNING jobs with expired leases and recovers each.
// Exported so it can be called directly in tests and the product proof.
func (s *Scheduler) Sweep(ctx context.Context) error {
	now := time.Now().UTC()

	rows, err := s.pool.Query(ctx, `
		SELECT id, attempt_count, max_attempts, current_attempt_id
		FROM jobs
		WHERE state IN ('CLAIMED','RUNNING')
		  AND lease_expires_at < $1`,
		now,
	)
	if err != nil {
		return err
	}

	type candidate struct {
		id               uuid.UUID
		attemptCount     int
		maxAttempts      int
		currentAttemptID *uuid.UUID
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.attemptCount, &c.maxAttempts, &c.currentAttemptID); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	recovered := 0
	for _, c := range candidates {
		if err := s.recoverJob(ctx, c.id, c.currentAttemptID, c.attemptCount, c.maxAttempts, now); err != nil {
			s.log.Error().Err(err).Str("job_id", c.id.String()).Msg("failed to recover job")
		} else {
			recovered++
		}
	}

	if len(candidates) > 0 {
		s.log.Info().
			Int("candidates", len(candidates)).
			Int("recovered", recovered).
			Msg("recovery sweep complete")
	}
	return nil
}

// recoverJob handles a single expired-lease job in an atomic transaction.
// SELECT FOR UPDATE re-checks state to prevent double-recovery across concurrent sweeps.
func (s *Scheduler) recoverJob(
	ctx context.Context,
	jobID uuid.UUID,
	currentAttemptID *uuid.UUID,
	attemptCount, maxAttempts int,
	now time.Time,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Re-check under lock. If already recovered, lease_expires_at >= now or state changed.
	var state string
	err = tx.QueryRow(ctx, `
		SELECT state FROM jobs
		WHERE id = $1
		  AND state IN ('CLAIMED','RUNNING')
		  AND lease_expires_at < $2
		FOR UPDATE`,
		jobID, now,
	).Scan(&state)
	if err == pgx.ErrNoRows {
		return nil // already recovered by another goroutine or process
	}
	if err != nil {
		return err
	}

	// Abandon the current attempt if the job has one.
	if currentAttemptID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE job_attempts
			SET state = 'ABANDONED', finished_at = $1, updated_at = $1
			WHERE id = $2
			  AND state NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','ABANDONED')`,
			now, *currentAttemptID,
		); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, jobID, currentAttemptID,
			domain.EventTypeAbandoned, state, "ABANDONED", now); err != nil {
			return err
		}
	}

	// Apply retry policy.
	policy := domain.RetryPolicy{MaxAttempts: maxAttempts}
	if policy.ShouldRetry(attemptCount) {
		// Re-queue immediately; Layer 9 adds per-job backoff here.
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET state              = 'QUEUED',
			    lease_token        = NULL,
			    lease_expires_at   = NULL,
			    current_attempt_id = NULL,
			    eligible_at        = $1,
			    updated_at         = $1
			WHERE id = $2`,
			now, jobID,
		); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, jobID, nil,
			domain.EventTypeRequeued, state, "QUEUED", now); err != nil {
			return err
		}
		s.log.Warn().
			Str("job_id", jobID.String()).
			Int("attempt_count", attemptCount).
			Int("max_attempts", maxAttempts).
			Str("from_state", state).
			Msg("job recovered: re-queued after lease expiry")
	} else {
		// Retries exhausted — terminate the job.
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET state            = 'FAILED',
			    terminal_at      = $1,
			    lease_token      = NULL,
			    lease_expires_at = NULL,
			    updated_at       = $1
			WHERE id = $2`,
			now, jobID,
		); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, jobID, nil,
			domain.EventTypeFailed, state, "FAILED", now); err != nil {
			return err
		}
		s.log.Warn().
			Str("job_id", jobID.String()).
			Int("attempt_count", attemptCount).
			Int("max_attempts", maxAttempts).
			Str("from_state", state).
			Msg("job recovered: terminated after lease expiry, retries exhausted")
	}

	return tx.Commit(ctx)
}

func insertEvent(
	ctx context.Context,
	tx pgx.Tx,
	jobID uuid.UUID,
	attemptID *uuid.UUID,
	eventType domain.EventType,
	fromState, toState string,
	now time.Time,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO job_events
			(id, job_id, attempt_id, type, from_state, to_state, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		uuid.New(), jobID, attemptID, string(eventType), fromState, toState, now,
	)
	return err
}
