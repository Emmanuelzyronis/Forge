package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JobRepo implements store.JobStore against PostgreSQL.
type JobRepo struct {
	pool *pgxpool.Pool
}

// NewJobRepo creates a new JobRepo backed by pool.
func NewJobRepo(pool *pgxpool.Pool) *JobRepo {
	return &JobRepo{pool: pool}
}

func (r *JobRepo) Create(ctx context.Context, j *domain.Job) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO jobs
			(id, idempotency_key, kind, payload, state, priority, max_attempts,
			 attempt_count, timeout_secs, created_at, queued_at, eligible_at,
			 updated_at, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		j.ID, j.IdempotencyKey, j.Kind, j.Payload, string(j.State),
		j.Priority, j.MaxAttempts, j.AttemptCount, j.TimeoutSecs,
		j.CreatedAt, j.QueuedAt, j.EligibleAt, j.UpdatedAt, j.CorrelationID,
	)
	return err
}

func (r *JobRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id)
	return scanJob(row)
}

func (r *JobRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Job, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE idempotency_key = $1`, key)
	j, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return j, err
}

// ClaimNext is the core dispatch primitive (F-TDR-003). It executes atomically:
//  1. SELECT FOR UPDATE SKIP LOCKED — exactly one worker wins the row lock.
//  2. UPDATE job → CLAIMED, set lease_token, lease_expires_at, increment attempt_count.
//  3. INSERT job_attempt in CREATED state.
//  4. INSERT job_event CLAIMED (immutable, F-INV-010).
//
// Returns nil, nil, nil when no eligible QUEUED job is available.
// capabilities: if non-empty, only claim jobs whose kind is in the list.
func (r *JobRepo) ClaimNext(ctx context.Context, workerID uuid.UUID, leaseToken uuid.UUID, leaseDuration time.Duration, capabilities []string) (*domain.Job, *domain.ExecutionAttempt, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	leaseSecs := int64(leaseDuration.Seconds())

	var row pgx.Row
	if len(capabilities) == 0 {
		row = tx.QueryRow(ctx, `
			UPDATE jobs SET
				state            = 'CLAIMED',
				lease_token      = $1,
				lease_expires_at = NOW() + ($2 * interval '1 second'),
				attempt_count    = attempt_count + 1,
				updated_at       = NOW()
			WHERE id = (
				SELECT id FROM jobs
				WHERE state = 'QUEUED'
				  AND eligible_at <= NOW()
				ORDER BY priority DESC, queued_at ASC
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			RETURNING `+jobColumns,
			leaseToken, leaseSecs,
		)
	} else {
		row = tx.QueryRow(ctx, `
			UPDATE jobs SET
				state            = 'CLAIMED',
				lease_token      = $1,
				lease_expires_at = NOW() + ($2 * interval '1 second'),
				attempt_count    = attempt_count + 1,
				updated_at       = NOW()
			WHERE id = (
				SELECT id FROM jobs
				WHERE state = 'QUEUED'
				  AND eligible_at <= NOW()
				  AND kind = ANY($3)
				ORDER BY priority DESC, queued_at ASC
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			RETURNING `+jobColumns,
			leaseToken, leaseSecs, capabilities,
		)
	}

	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	attemptID := uuid.New()
	attempt := &domain.ExecutionAttempt{
		ID:         attemptID,
		JobID:      job.ID,
		WorkerID:   workerID,
		AttemptNum: job.AttemptCount, // already incremented by the UPDATE above
		State:      domain.AttemptStateCreated,
		LeaseToken: &leaseToken,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO job_attempts
			(id, job_id, attempt_num, worker_id, state, lease_token, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		attemptID, job.ID, attempt.AttemptNum, workerID,
		string(attempt.State), leaseToken, now, now,
	)
	if err != nil {
		return nil, nil, err
	}

	_, err = tx.Exec(ctx, `UPDATE jobs SET current_attempt_id = $1 WHERE id = $2`, attemptID, job.ID)
	if err != nil {
		return nil, nil, err
	}
	job.CurrentAttemptID = &attemptID

	// Emit the immutable CLAIMED event in the same transaction (F-INV-010).
	fromState := string(domain.JobStateQueued)
	toState := string(domain.JobStateClaimed)
	_, err = tx.Exec(ctx, `
		INSERT INTO job_events
			(id, job_id, attempt_id, worker_id, type, from_state, to_state,
			 occurred_at, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),$8)`,
		uuid.New(), job.ID, attemptID, workerID,
		string(domain.EventTypeClaimed),
		fromState, toState,
		job.CorrelationID,
	)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return job, attempt, nil
}

func (r *JobRepo) UpdateState(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, state domain.JobState) error {
	_, err := tx.Exec(ctx,
		`UPDATE jobs SET state = $1, updated_at = NOW() WHERE id = $2`,
		string(state), jobID,
	)
	return err
}

func (r *JobRepo) SetTerminal(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, state domain.JobState, now time.Time) error {
	_, err := tx.Exec(ctx,
		`UPDATE jobs SET
			state = $1, terminal_at = $2, updated_at = $2,
			lease_token = NULL, lease_expires_at = NULL
		WHERE id = $3`,
		string(state), now, jobID,
	)
	return err
}

func (r *JobRepo) UpdateLease(ctx context.Context, jobID uuid.UUID, leaseToken uuid.UUID, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE jobs SET lease_expires_at = $1, updated_at = NOW()
		 WHERE id = $2 AND lease_token = $3`,
		expiresAt, jobID, leaseToken,
	)
	return err
}

func (r *JobRepo) Heartbeat(ctx context.Context, jobID uuid.UUID, leaseToken uuid.UUID, now time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE jobs SET last_heartbeat_at = $1
		 WHERE id = $2 AND lease_token = $3 AND state IN ('CLAIMED','RUNNING')`,
		now, jobID, leaseToken,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return &domain.StaleLeaseError{Reason: "heartbeat rejected: token mismatch or job not in-flight"}
	}
	return nil
}

func (r *JobRepo) Requeue(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, eligibleAt time.Time) error {
	_, err := tx.Exec(ctx,
		`UPDATE jobs SET
			state = 'QUEUED',
			lease_token = NULL, lease_expires_at = NULL,
			current_attempt_id = NULL,
			eligible_at = $1,
			updated_at = NOW()
		WHERE id = $2`,
		eligibleAt, jobID,
	)
	return err
}

func (r *JobRepo) ListExpiredLeases(ctx context.Context, now time.Time) ([]*domain.Job, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+jobColumns+`
		FROM jobs
		WHERE state IN ('CLAIMED','RUNNING')
		  AND lease_expires_at < $1`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*domain.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (r *JobRepo) List(ctx context.Context, filter store.JobFilter) ([]*domain.Job, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	q := `SELECT ` + jobColumns + ` FROM jobs WHERE TRUE`
	args := []any{}
	idx := 1

	if filter.State != nil {
		q += fmt.Sprintf(" AND state = $%d", idx)
		args = append(args, string(*filter.State))
		idx++
	}
	if filter.Kind != nil {
		q += fmt.Sprintf(" AND kind = $%d", idx)
		args = append(args, *filter.Kind)
		idx++
	}
	if filter.CorrelationID != nil {
		q += fmt.Sprintf(" AND correlation_id = $%d", idx)
		args = append(args, *filter.CorrelationID)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*domain.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// jobColumns is the canonical SELECT list for scanJob. Order must match scanJob exactly.
const jobColumns = `
	id, idempotency_key, kind, payload, state, priority, max_attempts, attempt_count,
	timeout_secs, created_at, queued_at, eligible_at, updated_at, terminal_at,
	current_attempt_id, lease_token, lease_expires_at, last_heartbeat_at, correlation_id`

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (*domain.Job, error) {
	j := &domain.Job{}
	var state string
	err := row.Scan(
		&j.ID, &j.IdempotencyKey, &j.Kind, &j.Payload, &state,
		&j.Priority, &j.MaxAttempts, &j.AttemptCount,
		&j.TimeoutSecs, &j.CreatedAt, &j.QueuedAt, &j.EligibleAt, &j.UpdatedAt,
		&j.TerminalAt, &j.CurrentAttemptID, &j.LeaseToken, &j.LeaseExpiresAt,
		&j.LastHeartbeatAt, &j.CorrelationID,
	)
	if err != nil {
		return nil, err
	}
	j.State = domain.JobState(state)
	return j, nil
}
