package lifecycle

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Service manages job execution lifecycle transitions: Start, Heartbeat, Succeed, Fail.
// It holds the pool directly so it can issue cross-table atomic transactions across
// job_attempts, jobs, and job_events in a single commit (F-INV-010).
type Service struct {
	pool          *pgxpool.Pool
	leaseDuration time.Duration
	log           zerolog.Logger
}

func NewService(pool *pgxpool.Pool, leaseDuration time.Duration, log zerolog.Logger) *Service {
	return &Service{pool: pool, leaseDuration: leaseDuration, log: log}
}

// Start transitions the attempt CREATED→RUNNING and the job CLAIMED→RUNNING.
// Returns domain.StaleLeaseError if the lease token is invalid or states do not match.
func (s *Service) Start(ctx context.Context, attemptID, leaseToken uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE job_attempts
		SET state = 'RUNNING', started_at = $1, updated_at = $1
		WHERE id = $2 AND lease_token = $3 AND state = 'CREATED'`,
		now, attemptID, leaseToken,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return &domain.StaleLeaseError{Reason: "start: token mismatch or attempt not in CREATED state"}
	}

	var jobID, workerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT job_id, worker_id FROM job_attempts WHERE id = $1`, attemptID,
	).Scan(&jobID, &workerID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE jobs SET state = 'RUNNING', updated_at = $1 WHERE id = $2`,
		now, jobID,
	); err != nil {
		return err
	}

	fromState := string(domain.JobStateClaimed)
	toState := string(domain.JobStateRunning)
	aid := attemptID
	wid := workerID
	return appendCommit(ctx, tx, &domain.JobEvent{
		ID:         uuid.New(),
		JobID:      jobID,
		AttemptID:  &aid,
		WorkerID:   &wid,
		Type:       domain.EventTypeStarted,
		FromState:  &fromState,
		ToState:    &toState,
		OccurredAt: now,
	})
}

// JobHeartbeat extends lease_expires_at and refreshes last_heartbeat_at on an in-flight job.
// Extending the lease prevents the recovery scheduler from reclaiming jobs from healthy workers.
// Returns domain.StaleLeaseError if the token is invalid or the job is not in-flight.
func (s *Service) JobHeartbeat(ctx context.Context, jobID, leaseToken uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET lease_expires_at = $1, last_heartbeat_at = $2, updated_at = $2
		WHERE id = $3 AND lease_token = $4 AND state IN ('CLAIMED','RUNNING')`,
		now.Add(s.leaseDuration), now, jobID, leaseToken,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return &domain.StaleLeaseError{Reason: "heartbeat: token mismatch or job not in-flight"}
	}
	return nil
}

// Succeed transitions the attempt RUNNING→SUCCEEDED and the job RUNNING→SUCCEEDED (terminal).
func (s *Service) Succeed(ctx context.Context, attemptID, leaseToken uuid.UUID, result []byte, durationMS int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE job_attempts
		SET state = 'SUCCEEDED', finished_at = $1, result = $2, duration_ms = $3, updated_at = $1
		WHERE id = $4 AND lease_token = $5 AND state = 'RUNNING'`,
		now, result, durationMS, attemptID, leaseToken,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return &domain.StaleLeaseError{Reason: "succeed: token mismatch or attempt not RUNNING"}
	}

	var jobID, workerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT job_id, worker_id FROM job_attempts WHERE id = $1`, attemptID,
	).Scan(&jobID, &workerID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs SET
			state = 'SUCCEEDED', terminal_at = $1, updated_at = $1,
			lease_token = NULL, lease_expires_at = NULL
		WHERE id = $2`,
		now, jobID,
	); err != nil {
		return err
	}

	fromState := string(domain.JobStateRunning)
	toState := string(domain.JobStateSucceeded)
	aid := attemptID
	wid := workerID
	return appendCommit(ctx, tx, &domain.JobEvent{
		ID:         uuid.New(),
		JobID:      jobID,
		AttemptID:  &aid,
		WorkerID:   &wid,
		Type:       domain.EventTypeSucceeded,
		FromState:  &fromState,
		ToState:    &toState,
		OccurredAt: now,
	})
}

// Fail transitions the attempt RUNNING→FAILED and either re-queues or terminates the job
// depending on whether the retry policy allows another attempt (F-INV-005).
func (s *Service) Fail(ctx context.Context, attemptID, leaseToken uuid.UUID, reason string, detail []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE job_attempts
		SET state = 'FAILED', finished_at = $1, error_detail = $2, failure_detail = $3, updated_at = $1
		WHERE id = $4 AND lease_token = $5 AND state = 'RUNNING'`,
		now, reason, detail, attemptID, leaseToken,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return &domain.StaleLeaseError{Reason: "fail: token mismatch or attempt not RUNNING"}
	}

	var jobID, workerID uuid.UUID
	var maxAttempts, attemptCount int
	var correlationID *string
	if err := tx.QueryRow(ctx, `
		SELECT j.id, j.max_attempts, j.attempt_count, j.correlation_id, ja.worker_id
		FROM jobs j JOIN job_attempts ja ON ja.job_id = j.id
		WHERE ja.id = $1`,
		attemptID,
	).Scan(&jobID, &maxAttempts, &attemptCount, &correlationID, &workerID); err != nil {
		return err
	}

	policy := domain.RetryPolicy{MaxAttempts: maxAttempts}
	var evType domain.EventType
	var toState string

	if policy.ShouldRetry(attemptCount) {
		eligibleAt := policy.NextEligibleAt(attemptCount, now)
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET
				state = 'QUEUED', eligible_at = $1,
				lease_token = NULL, lease_expires_at = NULL,
				current_attempt_id = NULL, updated_at = $2
			WHERE id = $3`,
			eligibleAt, now, jobID,
		); err != nil {
			return err
		}
		evType = domain.EventTypeRequeued
		toState = string(domain.JobStateQueued)
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET
				state = 'FAILED', terminal_at = $1, updated_at = $1,
				lease_token = NULL, lease_expires_at = NULL
			WHERE id = $2`,
			now, jobID,
		); err != nil {
			return err
		}
		evType = domain.EventTypeFailed
		toState = string(domain.JobStateFailed)
	}

	fromState := string(domain.JobStateRunning)
	aid := attemptID
	wid := workerID
	return appendCommit(ctx, tx, &domain.JobEvent{
		ID:            uuid.New(),
		JobID:         jobID,
		AttemptID:     &aid,
		WorkerID:      &wid,
		Type:          evType,
		FromState:     &fromState,
		ToState:       &toState,
		OccurredAt:    now,
		CorrelationID: correlationID,
	})
}

// appendCommit writes an immutable event in tx and commits the transaction.
func appendCommit(ctx context.Context, tx pgx.Tx, e *domain.JobEvent) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_events
			(id, job_id, attempt_id, worker_id, type, from_state, to_state, occurred_at, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.ID, e.JobID, e.AttemptID, e.WorkerID,
		string(e.Type), e.FromState, e.ToState,
		e.OccurredAt, e.CorrelationID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
