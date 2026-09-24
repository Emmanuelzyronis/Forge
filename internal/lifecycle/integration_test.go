package lifecycle_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/lifecycle"
	"github.com/Emmanuelzyronis/forge/internal/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("FORGE_DATABASE_URL")
	if dbURL == "" {
		t.Skip("FORGE_DATABASE_URL not set; skipping lifecycle integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func insertWorker(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO workers (id, name, hostname, pid, state, capabilities, registered_at, last_heartbeat_at, updated_at)
		VALUES ($1, $2, 'test-host', 0, 'IDLE', '{}', NOW(), NOW(), NOW())`,
		id, "test-worker-"+id.String()[:8],
	)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	return id
}

func insertQueuedJob(t *testing.T, pool *pgxpool.Pool, maxAttempts int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO jobs (id, kind, payload, state, priority, max_attempts, attempt_count, timeout_secs,
		                  created_at, queued_at, eligible_at, updated_at)
		VALUES ($1, 'lifecycle-test', '{}', 'QUEUED', 0, $2, 0, 30, NOW(), NOW(), NOW(), NOW())`,
		id, maxAttempts,
	)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

// claimJob claims the next queued job for workerID and returns the job/attempt.
func claimJob(t *testing.T, pool *pgxpool.Pool, workerID uuid.UUID) (jobID, attemptID uuid.UUID, leaseToken uuid.UUID) {
	t.Helper()
	jobRepo := postgres.NewJobRepo(pool)
	lt := uuid.New()
	job, attempt, err := jobRepo.ClaimNext(context.Background(), workerID, lt, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if job == nil {
		t.Fatal("ClaimNext: no job available")
	}
	return job.ID, attempt.ID, *attempt.LeaseToken
}

func TestStart_TransitionsToRunning(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()
	svc := lifecycle.NewService(pool, 30*time.Second, zerolog.Nop())

	workerID := insertWorker(t, pool)
	insertQueuedJob(t, pool, 3)

	jobID, attemptID, leaseToken := claimJob(t, pool, workerID)

	if err := svc.Start(ctx, attemptID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var attemptState, jobState string
	pool.QueryRow(ctx, `SELECT state FROM job_attempts WHERE id = $1`, attemptID).Scan(&attemptState) //nolint:errcheck
	pool.QueryRow(ctx, `SELECT state FROM jobs WHERE id = $1`, jobID).Scan(&jobState)                 //nolint:errcheck

	if attemptState != "RUNNING" {
		t.Errorf("attempt state = %q; want RUNNING", attemptState)
	}
	if jobState != "RUNNING" {
		t.Errorf("job state = %q; want RUNNING", jobState)
	}
}

func TestJobHeartbeat_UpdatesHeartbeatAt(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()
	svc := lifecycle.NewService(pool, 30*time.Second, zerolog.Nop())

	workerID := insertWorker(t, pool)
	insertQueuedJob(t, pool, 3)

	jobID, attemptID, leaseToken := claimJob(t, pool, workerID)

	// Must be RUNNING before heartbeat is accepted.
	if err := svc.Start(ctx, attemptID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}

	before := time.Now()
	if err := svc.JobHeartbeat(ctx, jobID, leaseToken); err != nil {
		t.Fatalf("JobHeartbeat: %v", err)
	}

	var heartbeatAt time.Time
	pool.QueryRow(ctx, `SELECT last_heartbeat_at FROM jobs WHERE id = $1`, jobID).Scan(&heartbeatAt) //nolint:errcheck

	if heartbeatAt.IsZero() || heartbeatAt.Before(before) {
		t.Errorf("last_heartbeat_at not refreshed: got %v", heartbeatAt)
	}
}

func TestSucceed_MarksJobSucceeded(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()
	svc := lifecycle.NewService(pool, 30*time.Second, zerolog.Nop())

	workerID := insertWorker(t, pool)
	insertQueuedJob(t, pool, 3)

	jobID, attemptID, leaseToken := claimJob(t, pool, workerID)

	if err := svc.Start(ctx, attemptID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Succeed(ctx, attemptID, leaseToken, []byte(`{"status":"ok"}`), 42); err != nil {
		t.Fatalf("Succeed: %v", err)
	}

	var attemptState, jobState string
	var terminalAt *time.Time
	pool.QueryRow(ctx, `SELECT state FROM job_attempts WHERE id = $1`, attemptID).Scan(&attemptState) //nolint:errcheck
	pool.QueryRow(ctx, `SELECT state, terminal_at FROM jobs WHERE id = $1`, jobID).Scan(&jobState, &terminalAt) //nolint:errcheck

	if attemptState != "SUCCEEDED" {
		t.Errorf("attempt state = %q; want SUCCEEDED", attemptState)
	}
	if jobState != "SUCCEEDED" {
		t.Errorf("job state = %q; want SUCCEEDED", jobState)
	}
	if terminalAt == nil {
		t.Error("terminal_at is NULL; want a timestamp")
	}
}

func TestFail_WithRetry_RequeuesJob(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()
	svc := lifecycle.NewService(pool, 30*time.Second, zerolog.Nop())

	workerID := insertWorker(t, pool)
	// max_attempts=2 so attempt_count=1 after claim → ShouldRetry(1) → 1 < 2 → true
	insertQueuedJob(t, pool, 2)

	jobID, attemptID, leaseToken := claimJob(t, pool, workerID)

	if err := svc.Start(ctx, attemptID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Fail(ctx, attemptID, leaseToken, "transient error", nil); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	var jobState string
	pool.QueryRow(ctx, `SELECT state FROM jobs WHERE id = $1`, jobID).Scan(&jobState) //nolint:errcheck

	if jobState != "QUEUED" {
		t.Errorf("job state = %q; want QUEUED (requeued for retry)", jobState)
	}
}

func TestFail_NoRetry_MarksJobFailed(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()
	svc := lifecycle.NewService(pool, 30*time.Second, zerolog.Nop())

	workerID := insertWorker(t, pool)
	// max_attempts=1 so attempt_count=1 after claim → ShouldRetry(1) → 1 < 1 → false
	insertQueuedJob(t, pool, 1)

	jobID, attemptID, leaseToken := claimJob(t, pool, workerID)

	if err := svc.Start(ctx, attemptID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Fail(ctx, attemptID, leaseToken, "fatal error", nil); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	var jobState string
	var terminalAt *time.Time
	pool.QueryRow(ctx, `SELECT state, terminal_at FROM jobs WHERE id = $1`, jobID).Scan(&jobState, &terminalAt) //nolint:errcheck

	if jobState != "FAILED" {
		t.Errorf("job state = %q; want FAILED", jobState)
	}
	if terminalAt == nil {
		t.Error("terminal_at is NULL; want a timestamp")
	}
}
