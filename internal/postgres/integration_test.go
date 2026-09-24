package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("FORGE_DATABASE_URL")
	if url == "" {
		fmt.Println("FORGE_DATABASE_URL not set — skipping postgres integration tests")
		os.Exit(0)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgxpool.New: %v\n", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	schema, err := os.ReadFile("../../migrations/001_init.up.sql")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read migration: %v\n", err)
		os.Exit(1)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		fmt.Fprintf(os.Stderr, "apply schema: %v\n", err)
		os.Exit(1)
	}
	if _, err := pool.Exec(ctx,
		`TRUNCATE job_events, job_attempts, jobs, workers RESTART IDENTITY CASCADE`); err != nil {
		fmt.Fprintf(os.Stderr, "truncate: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func truncate(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`TRUNCATE job_events, job_attempts, jobs, workers RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func newTestWorker() *domain.Worker {
	now := time.Now().UTC()
	return &domain.Worker{
		ID:              uuid.New(),
		Name:            "worker-" + uuid.NewString(),
		Hostname:        "localhost",
		PID:             os.Getpid(),
		State:           domain.WorkerStateRegistered,
		Capabilities:    []string{},
		RegisteredAt:    now,
		LastHeartbeatAt: now,
		UpdatedAt:       now,
	}
}

func newTestJob(workerID uuid.UUID) *domain.Job {
	now := time.Now().UTC()
	return &domain.Job{
		ID:           uuid.New(),
		Kind:         "test.job",
		Payload:      []byte(`{"v":1}`),
		State:        domain.JobStateQueued,
		Priority:     0,
		MaxAttempts:  3,
		AttemptCount: 0,
		TimeoutSecs:  30,
		CreatedAt:    now,
		QueuedAt:     now,
		EligibleAt:   now,
		UpdatedAt:    now,
	}
}

func setupWorker(t *testing.T) *domain.Worker {
	t.Helper()
	w := newTestWorker()
	repo := postgres.NewWorkerRepo(testPool)
	if err := repo.Upsert(context.Background(), w); err != nil {
		t.Fatalf("Upsert worker: %v", err)
	}
	return w
}

// TestJobCreateAndGet verifies basic persistence round-trip.
func TestJobCreateAndGet(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	repo := postgres.NewJobRepo(testPool)

	job := newTestJob(w.ID)
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("ID: want %s, got %s", job.ID, got.ID)
	}
	if got.State != domain.JobStateQueued {
		t.Errorf("State: want QUEUED, got %s", got.State)
	}
	if got.Kind != "test.job" {
		t.Errorf("Kind: want test.job, got %s", got.Kind)
	}
}

// TestJobIdempotencyKeyUnique verifies the unique constraint on idempotency_key.
func TestJobIdempotencyKeyUnique(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	repo := postgres.NewJobRepo(testPool)

	key := uuid.NewString()
	job := newTestJob(w.ID)
	job.IdempotencyKey = &key
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup := newTestJob(w.ID)
	dup.IdempotencyKey = &key
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("expected error on duplicate idempotency key, got nil")
	}
}

// TestGetByIdempotencyKey verifies key-based lookup and nil return for missing key.
func TestGetByIdempotencyKey(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	repo := postgres.NewJobRepo(testPool)

	key := uuid.NewString()
	job := newTestJob(w.ID)
	job.IdempotencyKey = &key
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByIdempotencyKey(ctx, key)
	if err != nil {
		t.Fatalf("GetByIdempotencyKey: %v", err)
	}
	if got == nil || got.ID != job.ID {
		t.Errorf("want job %s, got %v", job.ID, got)
	}

	none, err := repo.GetByIdempotencyKey(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if none != nil {
		t.Errorf("expected nil for missing key, got %+v", none)
	}
}

// TestClaimNext verifies a single worker can claim a QUEUED job.
func TestClaimNext(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	repo := postgres.NewJobRepo(testPool)

	job := newTestJob(w.ID)
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	leaseToken := uuid.New()
	claimed, attempt, err := repo.ClaimNext(ctx, w.ID, leaseToken, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimed job, got nil")
	}
	if claimed.ID != job.ID {
		t.Errorf("claimed wrong job: want %s, got %s", job.ID, claimed.ID)
	}
	if claimed.State != domain.JobStateClaimed {
		t.Errorf("state: want CLAIMED, got %s", claimed.State)
	}
	if claimed.LeaseToken == nil || *claimed.LeaseToken != leaseToken {
		t.Errorf("lease_token mismatch")
	}
	if claimed.AttemptCount != 1 {
		t.Errorf("attempt_count: want 1, got %d", claimed.AttemptCount)
	}
	if attempt == nil {
		t.Fatal("expected attempt, got nil")
	}
	if attempt.State != domain.AttemptStateCreated {
		t.Errorf("attempt state: want CREATED, got %s", attempt.State)
	}
	if attempt.WorkerID != w.ID {
		t.Errorf("attempt worker_id mismatch")
	}
}

// TestClaimNextNoJob verifies nil,nil,nil is returned when the queue is empty.
func TestClaimNextNoJob(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	repo := postgres.NewJobRepo(testPool)

	claimed, attempt, err := repo.ClaimNext(ctx, w.ID, uuid.New(), 30*time.Second, nil)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if claimed != nil || attempt != nil {
		t.Errorf("expected nil, nil; got %v, %v", claimed, attempt)
	}
}

// TestConcurrentClaim is the core SKIP LOCKED proof.
// N goroutines race to claim a single QUEUED job; exactly one must win.
func TestConcurrentClaim(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := postgres.NewJobRepo(testPool)
	workerRepo := postgres.NewWorkerRepo(testPool)

	job := newTestJob(uuid.UUID{})
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const n = 10
	var winners int64
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			// Each goroutine registers its own worker (unique name).
			w := newTestWorker()
			if err := workerRepo.Upsert(ctx, w); err != nil {
				t.Errorf("Upsert worker: %v", err)
				return
			}
			claimed, _, err := repo.ClaimNext(ctx, w.ID, uuid.New(), 30*time.Second, nil)
			if err != nil {
				t.Errorf("ClaimNext error: %v", err)
				return
			}
			if claimed != nil {
				atomic.AddInt64(&winners, 1)
			}
		}()
	}

	wg.Wait()

	if winners != 1 {
		t.Errorf("SKIP LOCKED violation: %d goroutines claimed the same job (want exactly 1)", winners)
	}
}

// TestHeartbeatWrongToken verifies StaleLeaseError is returned for a wrong token.
func TestHeartbeatWrongToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	repo := postgres.NewJobRepo(testPool)

	job := newTestJob(w.ID)
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	leaseToken := uuid.New()
	if _, _, err := repo.ClaimNext(ctx, w.ID, leaseToken, 30*time.Second, nil); err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}

	err := repo.Heartbeat(ctx, job.ID, uuid.New(), time.Now().UTC())
	if err == nil {
		t.Fatal("expected StaleLeaseError for wrong token, got nil")
	}
	var stale *domain.StaleLeaseError
	if !errors.As(err, &stale) {
		t.Errorf("expected *StaleLeaseError, got %T: %v", err, err)
	}
}

// TestEventAppendAndList verifies immutable event append + retrieval.
func TestEventAppendAndList(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	w := setupWorker(t)
	jobRepo := postgres.NewJobRepo(testPool)
	eventRepo := postgres.NewEventRepo(testPool)

	job := newTestJob(w.ID)
	if err := jobRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create job: %v", err)
	}

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	from := ""
	to := string(domain.JobStateQueued)
	event := &domain.JobEvent{
		ID:         uuid.New(),
		JobID:      job.ID,
		Type:       domain.EventTypeSubmitted,
		FromState:  &from,
		ToState:    &to,
		OccurredAt: time.Now().UTC(),
	}
	if err := eventRepo.Append(ctx, tx, event); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	events, err := eventRepo.ListByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListByJobID: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Type != domain.EventTypeSubmitted {
		t.Errorf("event type: want SUBMITTED, got %s", events[0].Type)
	}
}

// TestWorkerUpsertAndGet verifies worker registration idempotency.
func TestWorkerUpsertAndGet(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := postgres.NewWorkerRepo(testPool)

	w := newTestWorker()
	if err := repo.Upsert(ctx, w); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Second upsert with same name should update, not fail.
	w2 := *w
	w2.PID = 99999
	if err := repo.Upsert(ctx, &w2); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := repo.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PID != 99999 {
		t.Errorf("PID: want 99999, got %d", got.PID)
	}
}
