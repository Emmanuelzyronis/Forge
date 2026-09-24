package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/api"
	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// stubJobStore satisfies store.JobStore for handler tests.
type stubJobStore struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.Job, error)
	listFn    func(ctx context.Context, filter store.JobFilter) ([]*domain.Job, error)
}

func (s *stubJobStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	return s.getByIDFn(ctx, id)
}
func (s *stubJobStore) List(ctx context.Context, filter store.JobFilter) ([]*domain.Job, error) {
	return s.listFn(ctx, filter)
}
func (s *stubJobStore) Create(_ context.Context, _ *domain.Job) error                  { panic("not called") }
func (s *stubJobStore) GetByIdempotencyKey(_ context.Context, _ string) (*domain.Job, error) {
	panic("not called")
}
func (s *stubJobStore) ClaimNext(_ context.Context, _, _ uuid.UUID, _ time.Duration, _ []string) (*domain.Job, *domain.ExecutionAttempt, error) {
	panic("not called")
}
func (s *stubJobStore) UpdateState(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.JobState) error {
	panic("not called")
}
func (s *stubJobStore) SetTerminal(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.JobState, _ time.Time) error {
	panic("not called")
}
func (s *stubJobStore) UpdateLease(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error {
	panic("not called")
}
func (s *stubJobStore) Heartbeat(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error {
	panic("not called")
}
func (s *stubJobStore) Requeue(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ time.Time) error {
	panic("not called")
}
func (s *stubJobStore) ListExpiredLeases(_ context.Context, _ time.Time) ([]*domain.Job, error) {
	panic("not called")
}

func TestGetJobHandler_InvalidUUID(t *testing.T) {
	h := api.GetJobHandler(&stubJobStore{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Job, error) { return nil, nil },
	}, zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/jobs/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGetJobHandler_NotFound(t *testing.T) {
	h := api.GetJobHandler(&stubJobStore{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Job, error) {
			return nil, errors.New("not found")
		},
	}, zerolog.Nop())

	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestListJobsHandler_Empty(t *testing.T) {
	h := api.ListJobsHandler(&stubJobStore{
		listFn: func(_ context.Context, _ store.JobFilter) ([]*domain.Job, error) {
			return nil, nil // store returns nil slice
		},
	}, zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var body json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if string(body) == "null" {
		t.Error("response body must be [] not null when store returns nil slice")
	}
}

func TestListJobsHandler_StateFilter(t *testing.T) {
	var capturedFilter store.JobFilter
	h := api.ListJobsHandler(&stubJobStore{
		listFn: func(_ context.Context, filter store.JobFilter) ([]*domain.Job, error) {
			capturedFilter = filter
			return []*domain.Job{}, nil
		},
	}, zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/jobs?state=QUEUED&limit=10", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if capturedFilter.State == nil || *capturedFilter.State != domain.JobStateQueued {
		t.Errorf("want state filter QUEUED, got %v", capturedFilter.State)
	}
	if capturedFilter.Limit != 10 {
		t.Errorf("want limit 10, got %d", capturedFilter.Limit)
	}
}
