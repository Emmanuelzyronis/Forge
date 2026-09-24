package postgres

import (
	"context"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventRepo implements store.EventStore against PostgreSQL.
type EventRepo struct {
	pool *pgxpool.Pool
}

// NewEventRepo creates a new EventRepo backed by pool.
func NewEventRepo(pool *pgxpool.Pool) *EventRepo {
	return &EventRepo{pool: pool}
}

// Append writes an immutable event in tx. Must be called in the same transaction
// as the state change it records (F-INV-010).
func (r *EventRepo) Append(ctx context.Context, tx pgx.Tx, e *domain.JobEvent) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO job_events
			(id, job_id, attempt_id, worker_id, type, from_state, to_state,
			 metadata, occurred_at, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.ID, e.JobID, e.AttemptID, e.WorkerID,
		string(e.Type), e.FromState, e.ToState,
		e.Metadata, e.OccurredAt, e.CorrelationID,
	)
	return err
}

func (r *EventRepo) ListByJobID(ctx context.Context, jobID uuid.UUID) ([]*domain.JobEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, job_id, attempt_id, worker_id, type, from_state, to_state,
		       metadata, occurred_at, correlation_id
		FROM job_events
		WHERE job_id = $1
		ORDER BY occurred_at ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.JobEvent
	for rows.Next() {
		ev := &domain.JobEvent{}
		var evType string
		if err := rows.Scan(
			&ev.ID, &ev.JobID, &ev.AttemptID, &ev.WorkerID,
			&evType, &ev.FromState, &ev.ToState,
			&ev.Metadata, &ev.OccurredAt, &ev.CorrelationID,
		); err != nil {
			return nil, err
		}
		ev.Type = domain.EventType(evType)
		events = append(events, ev)
	}
	return events, rows.Err()
}
