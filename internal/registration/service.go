package registration

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type RegisterRequest struct {
	Name         string
	Hostname     string
	PID          int
	Capabilities []string
}

type Service struct {
	workers store.WorkerStore
	log     zerolog.Logger
}

func NewService(workers store.WorkerStore, log zerolog.Logger) *Service {
	return &Service{workers: workers, log: log}
}

// Register upserts the worker by name. Same name on restart → same record, updated PID.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*domain.Worker, error) {
	now := time.Now().UTC()
	caps := req.Capabilities
	if caps == nil {
		caps = []string{}
	}
	w := &domain.Worker{
		ID:              uuid.New(),
		Name:            req.Name,
		Hostname:        req.Hostname,
		PID:             req.PID,
		State:           domain.WorkerStateIdle,
		Capabilities:    caps,
		RegisteredAt:    now,
		LastHeartbeatAt: now,
		UpdatedAt:       now,
	}
	if err := s.workers.Upsert(ctx, w); err != nil {
		return nil, err
	}
	// Re-fetch to get the actual DB-side record (ON CONFLICT may have updated an existing row).
	fetched, err := s.workers.GetByID(ctx, w.ID)
	if err != nil {
		s.log.Warn().Err(err).Msg("could not re-fetch worker after upsert; returning local copy")
		return w, nil
	}
	s.log.Info().Str("worker_id", fetched.ID.String()).Str("name", fetched.Name).Msg("worker registered")
	return fetched, nil
}

// Heartbeat refreshes the worker's last_heartbeat_at timestamp.
func (s *Service) Heartbeat(ctx context.Context, workerID uuid.UUID) error {
	return s.workers.Heartbeat(ctx, workerID, time.Now().UTC())
}

// Offline marks the worker as OFFLINE on clean shutdown.
func (s *Service) Offline(ctx context.Context, workerID uuid.UUID) error {
	s.log.Info().Str("worker_id", workerID.String()).Msg("worker going offline")
	return s.workers.MarkOffline(ctx, workerID)
}
