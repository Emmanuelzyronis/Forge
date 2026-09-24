package dispatch

import (
	"context"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// ClaimResult carries what the worker needs to execute the claimed job.
type ClaimResult struct {
	Job        *domain.Job
	Attempt    *domain.ExecutionAttempt
	LeaseToken uuid.UUID
}

// Service handles the dispatch (claim) operation.
type Service struct {
	jobs          store.JobStore
	leaseDuration time.Duration
	log           zerolog.Logger
}

func NewService(jobs store.JobStore, leaseDuration time.Duration, log zerolog.Logger) *Service {
	return &Service{jobs: jobs, leaseDuration: leaseDuration, log: log}
}

// ClaimNext atomically claims the highest-priority eligible QUEUED job for workerID.
// Generates a fresh lease_token for this claim. Returns nil when no job is available.
func (s *Service) ClaimNext(ctx context.Context, workerID uuid.UUID, capabilities []string) (*ClaimResult, error) {
	leaseToken := uuid.New()
	job, attempt, err := s.jobs.ClaimNext(ctx, workerID, leaseToken, s.leaseDuration, capabilities)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	s.log.Info().
		Str("job_id", job.ID.String()).
		Str("worker_id", workerID.String()).
		Str("kind", job.Kind).
		Int("attempt_num", attempt.AttemptNum).
		Msg("job claimed")
	return &ClaimResult{Job: job, Attempt: attempt, LeaseToken: leaseToken}, nil
}
