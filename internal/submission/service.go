package submission

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
)

const (
	defaultMaxAttempts = 3
	defaultTimeoutSecs = 300
	maxAttemptsLimit   = 10
	maxTimeoutSecs     = 3600
)

// SubmitRequest carries all caller-supplied fields for a new job.
type SubmitRequest struct {
	IdempotencyKey string
	Kind           string
	Payload        json.RawMessage
	Priority       int
	MaxAttempts    int
	TimeoutSecs    int
	EligibleAt     *time.Time
	CorrelationID  string
}

// SubmitResult holds the job and whether it was newly created.
type SubmitResult struct {
	Job     *domain.Job
	Created bool // true → HTTP 201; false → HTTP 200 (idempotent return)
}

// Service handles job submission with idempotency and validation.
type Service struct {
	jobs store.JobStore
	log  zerolog.Logger
}

func NewService(jobs store.JobStore, log zerolog.Logger) *Service {
	return &Service{jobs: jobs, log: log}
}

// Submit validates, deduplicates, and persists a new job.
// If a job with the same IdempotencyKey already exists, that job is returned
// with Created=false. The caller maps Created to HTTP 201 or 200 respectively.
func (s *Service) Submit(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	if err := validate(req); err != nil {
		return nil, err
	}

	// Check for an existing job before attempting insert.
	existing, err := s.jobs.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		jobID := existing.ID.String()
		s.log.Info().Str("job_id", jobID).
			Str("idempotency_key", req.IdempotencyKey).
			Msg("idempotent submit: returning existing job")
		return &SubmitResult{Job: existing, Created: false}, nil
	}

	now := time.Now().UTC()
	eligibleAt := now
	if req.EligibleAt != nil {
		eligibleAt = req.EligibleAt.UTC()
	}
	if req.MaxAttempts == 0 {
		req.MaxAttempts = defaultMaxAttempts
	}
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = defaultTimeoutSecs
	}

	idempotencyKey := req.IdempotencyKey
	correlationID := req.CorrelationID

	job := &domain.Job{
		ID:             uuid.New(),
		IdempotencyKey: &idempotencyKey,
		Kind:           req.Kind,
		Payload:        req.Payload,
		State:          domain.JobStateQueued,
		Priority:       req.Priority,
		MaxAttempts:    req.MaxAttempts,
		AttemptCount:   0,
		TimeoutSecs:    req.TimeoutSecs,
		CreatedAt:      now,
		QueuedAt:       now,
		EligibleAt:     eligibleAt,
		UpdatedAt:      now,
		CorrelationID:  &correlationID,
	}

	if err := s.jobs.Create(ctx, job); err != nil {
		// Race: another concurrent request inserted the same idempotency key
		// between our check and our insert. Re-fetch and return that job.
		if isDuplicateKeyError(err) {
			existing, err2 := s.jobs.GetByIdempotencyKey(ctx, req.IdempotencyKey)
			if err2 != nil {
				return nil, err2
			}
			if existing != nil {
				return &SubmitResult{Job: existing, Created: false}, nil
			}
		}
		return nil, err
	}

	s.log.Info().Str("job_id", job.ID.String()).
		Str("kind", job.Kind).
		Str("idempotency_key", req.IdempotencyKey).
		Msg("job submitted")

	return &SubmitResult{Job: job, Created: true}, nil
}

func validate(req SubmitRequest) error {
	if req.IdempotencyKey == "" {
		return &ValidationError{Field: "idempotency_key", Message: "required"}
	}
	if req.Kind == "" {
		return &ValidationError{Field: "kind", Message: "required"}
	}
	if len(req.Payload) > 0 && !json.Valid(req.Payload) {
		return &ValidationError{Field: "payload", Message: "must be valid JSON"}
	}
	if req.MaxAttempts < 0 || req.MaxAttempts > maxAttemptsLimit {
		return &ValidationError{Field: "max_attempts", Message: "must be between 0 and 10"}
	}
	if req.TimeoutSecs < 0 || req.TimeoutSecs > maxTimeoutSecs {
		return &ValidationError{Field: "timeout_secs", Message: "must be between 0 and 3600"}
	}
	return nil
}

func isDuplicateKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
