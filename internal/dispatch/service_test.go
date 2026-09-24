package dispatch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/dispatch"
	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// stubJobStore is a minimal store.JobStore for dispatch tests.
type stubJobStore struct {
	job      *domain.Job
	attempt  *domain.ExecutionAttempt
	claimErr error
}

func (s *stubJobStore) ClaimNext(_ context.Context, _ uuid.UUID, leaseToken uuid.UUID, _ time.Duration, _ []string) (*domain.Job, *domain.ExecutionAttempt, error) {
	if s.claimErr != nil {
		return nil, nil, s.claimErr
	}
	if s.job == nil {
		return nil, nil, nil
	}
	j := *s.job
	j.LeaseToken = &leaseToken
	return &j, s.attempt, nil
}

func (s *stubJobStore) Create(_ context.Context, _ *domain.Job) error                                                    { return nil }
func (s *stubJobStore) GetByID(_ context.Context, _ uuid.UUID) (*domain.Job, error)                                      { return nil, nil }
func (s *stubJobStore) GetByIdempotencyKey(_ context.Context, _ string) (*domain.Job, error)                             { return nil, nil }
func (s *stubJobStore) UpdateState(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.JobState) error                   { return nil }
func (s *stubJobStore) SetTerminal(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.JobState, _ time.Time) error      { return nil }
func (s *stubJobStore) UpdateLease(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error                      { return nil }
func (s *stubJobStore) Heartbeat(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error                        { return nil }
func (s *stubJobStore) Requeue(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ time.Time) error                             { return nil }
func (s *stubJobStore) ListExpiredLeases(_ context.Context, _ time.Time) ([]*domain.Job, error)                         { return nil, nil }

var _ store.JobStore = (*stubJobStore)(nil)

func newSvc(st *stubJobStore) *dispatch.Service {
	return dispatch.NewService(st, 30*time.Second, zerolog.Nop())
}

func TestClaimNextNoJob(t *testing.T) {
	svc := newSvc(&stubJobStore{})
	result, err := svc.ClaimNext(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if result != nil {
		t.Errorf("want nil result when queue empty, got %+v", result)
	}
}

func TestClaimNextReturnsJob(t *testing.T) {
	now := time.Now().UTC()
	leaseExp := now.Add(30 * time.Second)
	workerID := uuid.New()
	job := &domain.Job{
		ID:             uuid.New(),
		Kind:           "test.job",
		Payload:        []byte(`{"v":1}`),
		State:          domain.JobStateQueued,
		MaxAttempts:    3,
		AttemptCount:   1,
		TimeoutSecs:    30,
		CreatedAt:      now,
		QueuedAt:       now,
		EligibleAt:     now,
		UpdatedAt:      now,
		LeaseExpiresAt: &leaseExp,
	}
	attempt := &domain.ExecutionAttempt{
		ID:         uuid.New(),
		JobID:      job.ID,
		WorkerID:   workerID,
		AttemptNum: 1,
		State:      domain.AttemptStateCreated,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	svc := newSvc(&stubJobStore{job: job, attempt: attempt})
	result, err := svc.ClaimNext(context.Background(), workerID, nil)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if result == nil {
		t.Fatal("expected claim result, got nil")
	}
	if result.Job.ID != job.ID {
		t.Errorf("job ID: want %s, got %s", job.ID, result.Job.ID)
	}
	if result.Attempt.ID != attempt.ID {
		t.Errorf("attempt ID mismatch")
	}
	if result.LeaseToken == (uuid.UUID{}) {
		t.Error("LeaseToken should not be zero")
	}
}

func TestClaimNextPropagatesError(t *testing.T) {
	boom := errors.New("db down")
	svc := newSvc(&stubJobStore{claimErr: boom})
	_, err := svc.ClaimNext(context.Background(), uuid.New(), nil)
	if !errors.Is(err, boom) {
		t.Errorf("want db error, got %v", err)
	}
}

func TestClaimNextGeneratesUniqueLeaseTokens(t *testing.T) {
	now := time.Now().UTC()
	leaseExp := now.Add(30 * time.Second)
	workerID := uuid.New()
	job := &domain.Job{
		ID: uuid.New(), Kind: "test", State: domain.JobStateQueued,
		CreatedAt: now, QueuedAt: now, EligibleAt: now, UpdatedAt: now,
		LeaseExpiresAt: &leaseExp,
	}
	attempt := &domain.ExecutionAttempt{
		ID: uuid.New(), JobID: job.ID, WorkerID: workerID, AttemptNum: 1,
		State: domain.AttemptStateCreated, CreatedAt: now, UpdatedAt: now,
	}
	svc := newSvc(&stubJobStore{job: job, attempt: attempt})

	r1, _ := svc.ClaimNext(context.Background(), workerID, nil)
	r2, _ := svc.ClaimNext(context.Background(), workerID, nil)

	if r1 == nil || r2 == nil {
		t.Fatal("both claims should succeed")
	}
	if r1.LeaseToken == r2.LeaseToken {
		t.Error("lease tokens must be unique across claims")
	}
}
