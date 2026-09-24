package postgres

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkerRepo implements store.WorkerStore against PostgreSQL.
type WorkerRepo struct {
	pool *pgxpool.Pool
}

// NewWorkerRepo creates a new WorkerRepo backed by pool.
func NewWorkerRepo(pool *pgxpool.Pool) *WorkerRepo {
	return &WorkerRepo{pool: pool}
}

// Upsert inserts or updates a worker by name (idempotent registration).
// On conflict the existing id is kept and written back into w.ID so callers
// always have the stable, DB-authoritative identity after the call returns.
// This enforces the "same worker_name → same worker_id across restarts" invariant.
func (r *WorkerRepo) Upsert(ctx context.Context, w *domain.Worker) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO workers
			(id, name, hostname, pid, state, capabilities,
			 registered_at, last_heartbeat_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (name) DO UPDATE SET
			hostname         = EXCLUDED.hostname,
			pid              = EXCLUDED.pid,
			state            = EXCLUDED.state,
			capabilities     = EXCLUDED.capabilities,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			updated_at       = EXCLUDED.updated_at
		RETURNING id`,
		w.ID, w.Name, w.Hostname, w.PID, string(w.State),
		w.Capabilities, w.RegisteredAt, w.LastHeartbeatAt, w.UpdatedAt,
	).Scan(&w.ID)
}

func (r *WorkerRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Worker, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, hostname, pid, state, capabilities,
		       registered_at, last_heartbeat_at, updated_at, last_job_id
		FROM workers WHERE id = $1`, id)
	return scanWorker(row)
}

func (r *WorkerRepo) Heartbeat(ctx context.Context, workerID uuid.UUID, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workers SET last_heartbeat_at = $1, updated_at = $1 WHERE id = $2`,
		now, workerID,
	)
	return err
}

func (r *WorkerRepo) MarkStale(ctx context.Context, workerID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workers SET state = 'STALE', updated_at = NOW() WHERE id = $1`,
		workerID,
	)
	return err
}

func (r *WorkerRepo) ListActive(ctx context.Context) ([]*domain.Worker, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, hostname, pid, state, capabilities,
		       registered_at, last_heartbeat_at, updated_at, last_job_id
		FROM workers WHERE state NOT IN ('STALE','OFFLINE')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workers []*domain.Worker
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

func (r *WorkerRepo) MarkOffline(ctx context.Context, workerID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workers SET state = 'OFFLINE', updated_at = NOW() WHERE id = $1`,
		workerID,
	)
	return err
}

var _ store.WorkerStore = (*WorkerRepo)(nil)

func scanWorker(row rowScanner) (*domain.Worker, error) {
	w := &domain.Worker{}
	var state string
	err := row.Scan(
		&w.ID, &w.Name, &w.Hostname, &w.PID, &state, &w.Capabilities,
		&w.RegisteredAt, &w.LastHeartbeatAt, &w.UpdatedAt, &w.LastJobID,
	)
	if err != nil {
		return nil, err
	}
	w.State = domain.WorkerState(state)
	return w, nil
}
