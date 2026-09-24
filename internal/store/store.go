package store

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JobStore handles job persistence and the SKIP LOCKED claim primitive.
type JobStore interface {
	Create(ctx context.Context, job *domain.Job) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Job, error)
	// ClaimNext atomically claims the highest-priority eligible QUEUED job.
	// Opens its own transaction. Returns nil, nil, nil when no job is available.
	// Creates the ExecutionAttempt and emits a CLAIMED JobEvent in the same
	// transaction (F-INV-006, F-INV-010).
	ClaimNext(ctx context.Context, workerID uuid.UUID, leaseToken uuid.UUID, leaseDuration time.Duration, capabilities []string) (*domain.Job, *domain.ExecutionAttempt, error)
	UpdateState(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, state domain.JobState) error
	SetTerminal(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, state domain.JobState, now time.Time) error
	UpdateLease(ctx context.Context, jobID uuid.UUID, leaseToken uuid.UUID, expiresAt time.Time) error
	Heartbeat(ctx context.Context, jobID uuid.UUID, leaseToken uuid.UUID, now time.Time) error
	Requeue(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, eligibleAt time.Time) error
	ListExpiredLeases(ctx context.Context, now time.Time) ([]*domain.Job, error)
}

// AttemptStore handles execution attempt persistence.
type AttemptStore interface {
	Create(ctx context.Context, tx pgx.Tx, attempt *domain.ExecutionAttempt) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ExecutionAttempt, error)
	UpdateState(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, state domain.AttemptState) error
	Complete(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, result []byte, durationMS int64) error
	Fail(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, state domain.AttemptState, reason string, detail []byte) error
}

// WorkerStore handles worker registration and heartbeat.
type WorkerStore interface {
	Upsert(ctx context.Context, worker *domain.Worker) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Worker, error)
	Heartbeat(ctx context.Context, workerID uuid.UUID, now time.Time) error
	MarkStale(ctx context.Context, workerID uuid.UUID) error
	MarkOffline(ctx context.Context, workerID uuid.UUID) error
	ListActive(ctx context.Context) ([]*domain.Worker, error)
}

// EventStore appends immutable job lifecycle events (F-INV-010).
type EventStore interface {
	Append(ctx context.Context, tx pgx.Tx, event *domain.JobEvent) error
	ListByJobID(ctx context.Context, jobID uuid.UUID) ([]*domain.JobEvent, error)
}

// Stores bundles all four repositories.
type Stores struct {
	Jobs     JobStore
	Attempts AttemptStore
	Workers  WorkerStore
	Events   EventStore
}

// Pool is an alias for the pgxpool type used by postgres implementations.
type Pool = pgxpool.Pool
