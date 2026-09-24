package postgres

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AttemptRepo implements store.AttemptStore against PostgreSQL.
type AttemptRepo struct {
	pool *pgxpool.Pool
}

// NewAttemptRepo creates a new AttemptRepo backed by pool.
func NewAttemptRepo(pool *pgxpool.Pool) *AttemptRepo {
	return &AttemptRepo{pool: pool}
}

func (r *AttemptRepo) Create(ctx context.Context, tx pgx.Tx, a *domain.ExecutionAttempt) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO job_attempts
			(id, job_id, attempt_num, worker_id, state, lease_token, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		a.ID, a.JobID, a.AttemptNum, a.WorkerID,
		string(a.State), a.LeaseToken, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (r *AttemptRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ExecutionAttempt, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, job_id, attempt_num, worker_id, state, lease_token,
		       started_at, finished_at, error_detail, failure_detail,
		       result, duration_ms, created_at, updated_at
		FROM job_attempts WHERE id = $1`, id)
	return scanAttempt(row)
}

func (r *AttemptRepo) UpdateState(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, state domain.AttemptState) error {
	_, err := tx.Exec(ctx,
		`UPDATE job_attempts SET state = $1, updated_at = NOW() WHERE id = $2`,
		string(state), attemptID,
	)
	return err
}

func (r *AttemptRepo) Complete(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, result []byte, durationMS int64) error {
	now := time.Now().UTC()
	_, err := tx.Exec(ctx, `
		UPDATE job_attempts SET
			state = 'SUCCEEDED', finished_at = $1, result = $2,
			duration_ms = $3, updated_at = $1
		WHERE id = $4`,
		now, result, durationMS, attemptID,
	)
	return err
}

func (r *AttemptRepo) Fail(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, state domain.AttemptState, reason string, detail []byte) error {
	now := time.Now().UTC()
	_, err := tx.Exec(ctx, `
		UPDATE job_attempts SET
			state = $1, finished_at = $2, error_detail = $3,
			failure_detail = $4, updated_at = $2
		WHERE id = $5`,
		string(state), now, reason, detail, attemptID,
	)
	return err
}

func scanAttempt(row rowScanner) (*domain.ExecutionAttempt, error) {
	a := &domain.ExecutionAttempt{}
	var state string
	err := row.Scan(
		&a.ID, &a.JobID, &a.AttemptNum, &a.WorkerID, &state, &a.LeaseToken,
		&a.StartedAt, &a.FinishedAt, &a.ErrorDetail, &a.FailureDetail,
		&a.Result, &a.DurationMS, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.State = domain.AttemptState(state)
	return a, nil
}
