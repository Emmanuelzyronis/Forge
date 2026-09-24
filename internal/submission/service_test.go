package submission_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/Emmanuelzyronis/forge/internal/submission"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// memJobStore is a minimal in-memory implementation of store.JobStore for unit tests.
type memJobStore struct {
	mu     sync.Mutex
	byKey  map[string]*domain.Job
	byID   map[uuid.UUID]*domain.Job
}

func newMemJobStore() *memJobStore {
	return &memJobStore{
		byKey: make(map[string]*domain.Job),
		byID:  make(map[uuid.UUID]*domain.Job),
	}
}

func (m *memJobStore) Create(_ context.Context, j *domain.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var key string
	if j.IdempotencyKey != nil {
		key = *j.IdempotencyKey
	}
	if _, exists := m.byKey[key]; exists {
		return errors.New("duplicate idempotency key")
	}
	cp := *j
	m.byKey[key] = &cp
	m.byID[j.ID] = &cp
	return nil
}

func (m *memJobStore) GetByIdempotencyKey(_ context.Context, key string) (*domain.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.byKey[key]
	if !ok {
		return nil, nil
	}
	cp := *j
	return &cp, nil
}

func (m *memJobStore) GetByID(_ context.Context, id uuid.UUID) (*domain.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.byID[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	cp := *j
	return &cp, nil
}

// Stub remaining store.JobStore methods.
func (m *memJobStore) ClaimNext(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Duration, _ []string) (*domain.Job, *domain.ExecutionAttempt, error) {
	return nil, nil, nil
}
func (m *memJobStore) UpdateState(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.JobState) error {
	return nil
}
func (m *memJobStore) SetTerminal(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.JobState, _ time.Time) error {
	return nil
}
func (m *memJobStore) UpdateLease(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error {
	return nil
}
func (m *memJobStore) Heartbeat(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error {
	return nil
}
func (m *memJobStore) Requeue(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ time.Time) error {
	return nil
}
func (m *memJobStore) ListExpiredLeases(_ context.Context, _ time.Time) ([]*domain.Job, error) {
	return nil, nil
}

// Compile-time interface check.
var _ store.JobStore = (*memJobStore)(nil)

func newService(st *memJobStore) *submission.Service {
	return submission.NewService(st, zerolog.Nop())
}

func validRequest() submission.SubmitRequest {
	return submission.SubmitRequest{
		IdempotencyKey: "key-" + uuid.NewString(),
		Kind:           "test.job",
		Payload:        json.RawMessage(`{"v":1}`),
		MaxAttempts:    3,
		TimeoutSecs:    30,
	}
}

func TestSubmitCreatesJob(t *testing.T) {
	svc := newService(newMemJobStore())
	req := validRequest()

	result, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Created {
		t.Error("want Created=true for new job")
	}
	if result.Job.IdempotencyKey == nil || *result.Job.IdempotencyKey != req.IdempotencyKey {
		t.Errorf("idempotency_key mismatch: want %s", req.IdempotencyKey)
	}
	if result.Job.State != domain.JobStateQueued {
		t.Errorf("state: want QUEUED, got %s", result.Job.State)
	}
	if result.Job.Kind != req.Kind {
		t.Errorf("kind: want %s, got %s", req.Kind, result.Job.Kind)
	}
}

func TestSubmitIdempotent(t *testing.T) {
	st := newMemJobStore()
	svc := newService(st)
	req := validRequest()

	first, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	if !first.Created {
		t.Error("first submit: want Created=true")
	}

	second, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("second Submit: %v", err)
	}
	if second.Created {
		t.Error("second submit: want Created=false")
	}
	if second.Job.ID != first.Job.ID {
		t.Errorf("idempotent submit returned different job ID: %s vs %s", first.Job.ID, second.Job.ID)
	}
}

func TestSubmitDefaultsApplied(t *testing.T) {
	svc := newService(newMemJobStore())

	req := submission.SubmitRequest{
		IdempotencyKey: "key-" + uuid.NewString(),
		Kind:           "test.job",
		// MaxAttempts and TimeoutSecs omitted — defaults applied
	}

	result, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Job.MaxAttempts != 3 {
		t.Errorf("MaxAttempts: want 3, got %d", result.Job.MaxAttempts)
	}
	if result.Job.TimeoutSecs != 300 {
		t.Errorf("TimeoutSecs: want 300, got %d", result.Job.TimeoutSecs)
	}
}

func TestSubmitEligibleAtCustom(t *testing.T) {
	svc := newService(newMemJobStore())
	future := time.Now().Add(10 * time.Minute)
	req := validRequest()
	req.EligibleAt = &future

	result, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Job.EligibleAt.Before(future.Add(-time.Second)) {
		t.Errorf("EligibleAt not set correctly: got %v, want ~%v", result.Job.EligibleAt, future)
	}
}

func TestSubmitValidationEmptyIdempotencyKey(t *testing.T) {
	svc := newService(newMemJobStore())
	req := validRequest()
	req.IdempotencyKey = ""

	_, err := svc.Submit(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error for empty idempotency_key")
	}
	var ve *submission.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
	if ve.Field != "idempotency_key" {
		t.Errorf("expected field idempotency_key, got %s", ve.Field)
	}
}

func TestSubmitValidationEmptyKind(t *testing.T) {
	svc := newService(newMemJobStore())
	req := validRequest()
	req.Kind = ""

	_, err := svc.Submit(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error for empty kind")
	}
	var ve *submission.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
	if ve.Field != "kind" {
		t.Errorf("expected field kind, got %s", ve.Field)
	}
}

func TestSubmitValidationInvalidPayload(t *testing.T) {
	svc := newService(newMemJobStore())
	req := validRequest()
	req.Payload = json.RawMessage(`not-json`)

	_, err := svc.Submit(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error for invalid JSON payload")
	}
	var ve *submission.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
}

func TestSubmitValidationMaxAttemptsExceeded(t *testing.T) {
	svc := newService(newMemJobStore())
	req := validRequest()
	req.MaxAttempts = 11

	_, err := svc.Submit(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error for max_attempts > 10")
	}
	var ve *submission.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
}

func TestSubmitValidationTimeoutExceeded(t *testing.T) {
	svc := newService(newMemJobStore())
	req := validRequest()
	req.TimeoutSecs = 3601

	_, err := svc.Submit(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error for timeout_secs > 3600")
	}
	var ve *submission.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
}
