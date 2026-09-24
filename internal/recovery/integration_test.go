package recovery_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/postgres"
	"github.com/Emmanuelzyronis/forge/internal/recovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

func setupIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("FORGE_DATABASE_URL")
	if dbURL == "" {
		t.Skip("FORGE_DATABASE_URL not set; skipping recovery integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`TRUNCATE job_events, job_attempts, jobs, workers RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func insertTestWorker(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO workers (id, name, hostname, pid, state, capabilities, registered_at, last_heartbeat_at, updated_at)
		VALUES ($1, $2, 'test-host', 0, 'IDLE', '{}', NOW(), NOW(), NOW())`,
		id, "recovery-worker-"+id.String()[:8],
	)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	return id
}

func insertTestJob(t *testing.T, pool *pgxpool.Pool, maxAttempts int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO jobs (id, kind, payload, state, priority, max_attempts, attempt_count, timeout_secs,
		                  created_at, queued_at, eligible_at, updated_at)
		VALUES ($1, 'recovery-test', '{}', 'QUEUED', 0, $2, 0, 30, NOW(), NOW(), NOW(), NOW())`,
		id, maxAttempts,
	)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func claimTestJob(t *testing.T, pool *pgxpool.Pool, workerID uuid.UUID) (jobID, attemptID uuid.UUID) {
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
	return job.ID, attempt.ID
}

func expireLease(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE jobs SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`,
		jobID,
	)
	if err != nil {
		t.Fatalf("expire lease: %v", err)
	}
}

func jobState(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID) string {
	t.Helper()
	var state string
	err := pool.QueryRow(context.Background(),
		`SELECT state FROM jobs WHERE id = $1`, jobID,
	).Scan(&state)
	if err != nil {
		t.Fatalf("read job state: %v", err)
	}
	return state
}

func attemptState(t *testing.T, pool *pgxpool.Pool, attemptID uuid.UUID) string {
	t.Helper()
	var state string
	err := pool.QueryRow(context.Background(),
		`SELECT state FROM job_attempts WHERE id = $1`, attemptID,
	).Scan(&state)
	if err != nil {
		t.Fatalf("read attempt state: %v", err)
	}
	return state
}

// TestRecoveryRequeuesClaimedJob: expired lease on a job with retries remaining → QUEUED + attempt ABANDONED.
func TestRecoveryRequeuesClaimedJob(t *testing.T) {
	pool := setupIntegrationPool(t)
	truncateTables(t, pool)
	ctx := context.Background()

	workerID := insertTestWorker(t, pool)
	insertTestJob(t, pool, 3) // max_attempts=3; attempt_count becomes 1 after claim → ShouldRetry(1) → true
	jobID, attemptID := claimTestJob(t, pool, workerID)
	expireLease(t, pool, jobID)

	sched := recovery.NewScheduler(pool, 10*time.Second, zerolog.Nop())
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if got := jobState(t, pool, jobID); got != "QUEUED" {
		t.Errorf("job state = %q; want QUEUED", got)
	}
	if got := attemptState(t, pool, attemptID); got != "ABANDONED" {
		t.Errorf("attempt state = %q; want ABANDONED", got)
	}
}

// TestRecoveryTerminatesExhaustedJob: expired lease on a job with no retries remaining → FAILED.
func TestRecoveryTerminatesExhaustedJob(t *testing.T) {
	pool := setupIntegrationPool(t)
	truncateTables(t, pool)
	ctx := context.Background()

	workerID := insertTestWorker(t, pool)
	insertTestJob(t, pool, 1) // max_attempts=1; attempt_count becomes 1 after claim → ShouldRetry(1) → false
	jobID, _ := claimTestJob(t, pool, workerID)
	expireLease(t, pool, jobID)

	sched := recovery.NewScheduler(pool, 10*time.Second, zerolog.Nop())
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if got := jobState(t, pool, jobID); got != "FAILED" {
		t.Errorf("job state = %q; want FAILED (retries exhausted)", got)
	}

	var terminalAt *time.Time
	pool.QueryRow(ctx, `SELECT terminal_at FROM jobs WHERE id = $1`, jobID).Scan(&terminalAt) //nolint:errcheck
	if terminalAt == nil {
		t.Error("terminal_at is NULL; want a timestamp")
	}
}

// TestRecoveryIgnoresHealthyJob: a job with a non-expired lease is left untouched.
func TestRecoveryIgnoresHealthyJob(t *testing.T) {
	pool := setupIntegrationPool(t)
	truncateTables(t, pool)
	ctx := context.Background()

	workerID := insertTestWorker(t, pool)
	insertTestJob(t, pool, 3)
	jobID, _ := claimTestJob(t, pool, workerID)

	// Extend the lease far into the future so the sweep cannot see it.
	_, err := pool.Exec(ctx,
		`UPDATE jobs SET lease_expires_at = NOW() + INTERVAL '1 hour' WHERE id = $1`, jobID)
	if err != nil {
		t.Fatalf("extend lease: %v", err)
	}

	sched := recovery.NewScheduler(pool, 10*time.Second, zerolog.Nop())
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if got := jobState(t, pool, jobID); got != "CLAIMED" {
		t.Errorf("job state = %q; want CLAIMED (healthy job should not be recovered)", got)
	}
}

// TestRecoveryNoDoubleRecovery: running two sweeps against one expired job yields exactly one ABANDONED event.
func TestRecoveryNoDoubleRecovery(t *testing.T) {
	pool := setupIntegrationPool(t)
	truncateTables(t, pool)
	ctx := context.Background()

	workerID := insertTestWorker(t, pool)
	insertTestJob(t, pool, 3) // retries remain so it re-queues; attempt is abandoned
	jobID, _ := claimTestJob(t, pool, workerID)
	expireLease(t, pool, jobID)

	sched := recovery.NewScheduler(pool, 10*time.Second, zerolog.Nop())

	// First sweep — should recover the job.
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("first Sweep: %v", err)
	}
	// Second sweep — job is now QUEUED, not CLAIMED/RUNNING → no-op.
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("second Sweep: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_events WHERE job_id = $1 AND type = 'ABANDONED'`, jobID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count ABANDONED events: %v", err)
	}
	if count != 1 {
		t.Errorf("ABANDONED event count = %d; want exactly 1 (no double-recovery)", count)
	}
}
