// Package verification proves FORGE's core invariants under realistic failure conditions.
// Each test requires a real PostgreSQL database (FORGE_DATABASE_URL) and skips without one.
// These tests cross package boundaries and cannot be expressed as unit tests.
package verification_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/lifecycle"
	"github.com/Emmanuelzyronis/forge/internal/postgres"
	"github.com/Emmanuelzyronis/forge/internal/recovery"
	"github.com/Emmanuelzyronis/forge/internal/submission"
	"github.com/Emmanuelzyronis/forge/internal/testhelp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func mustCreateWorker(t *testing.T, pool *pgxpool.Pool) *domain.Worker {
	t.Helper()
	now := time.Now().UTC()
	w := &domain.Worker{
		ID:              uuid.New(),
		Name:            "vtest-worker-" + uuid.New().String()[:8],
		Hostname:        "test-host",
		PID:             os.Getpid(),
		State:           domain.WorkerStateRegistered,
		Capabilities:    []string{},
		RegisteredAt:    now,
		LastHeartbeatAt: now,
		UpdatedAt:       now,
	}
	repo := postgres.NewWorkerRepo(pool)
	if err := repo.Upsert(context.Background(), w); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	return w
}

func mustCreateJob(t *testing.T, pool *pgxpool.Pool, maxAttempts int) *domain.Job {
	t.Helper()
	now := time.Now().UTC()
	job := &domain.Job{
		ID:          uuid.New(),
		Kind:        "vtest.task",
		Payload:     []byte(`{}`),
		State:       domain.JobStateQueued,
		Priority:    0,
		MaxAttempts: maxAttempts,
		TimeoutSecs: 30,
		CreatedAt:   now,
		QueuedAt:    now,
		EligibleAt:  now,
		UpdatedAt:   now,
	}
	repo := postgres.NewJobRepo(pool)
	if err := repo.Create(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func mustClaim(t *testing.T, pool *pgxpool.Pool, workerID uuid.UUID) (*domain.Job, *domain.ExecutionAttempt) {
	t.Helper()
	leaseToken := uuid.New()
	repo := postgres.NewJobRepo(pool)
	job, attempt, err := repo.ClaimNext(context.Background(), workerID, leaseToken, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if job == nil {
		t.Fatal("ClaimNext: no job available")
	}
	return job, attempt
}

func expireLease(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE jobs SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`, jobID)
	if err != nil {
		t.Fatalf("expire lease: %v", err)
	}
}

func jobState(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(context.Background(),
		`SELECT state FROM jobs WHERE id = $1`, jobID).Scan(&state); err != nil {
		t.Fatalf("read job state: %v", err)
	}
	return state
}

func countEvents(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM job_events WHERE job_id = $1 AND type = $2`,
		jobID, eventType).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// ── invariant tests ───────────────────────────────────────────────────────────

// TestInvariant_FullLifecycleEventTrail proves F-INV-010: every state transition
// in a successful job run emits an immutable event in the same transaction.
// Sequence: QUEUED → CLAIMED (ClaimNext) → RUNNING (Start) → SUCCEEDED (Succeed).
func TestInvariant_FullLifecycleEventTrail(t *testing.T) {
	pool := testhelp.OpenDB(t)
	testhelp.TruncateTables(t, pool)
	ctx := context.Background()

	w := mustCreateWorker(t, pool)
	mustCreateJob(t, pool, 3)
	job, attempt := mustClaim(t, pool, w.ID)

	leaseDuration := 30 * time.Second
	svc := lifecycle.NewService(pool, leaseDuration, zerolog.Nop())

	leaseToken := *job.LeaseToken

	if err := svc.Start(ctx, attempt.ID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Succeed(ctx, attempt.ID, leaseToken, []byte(`{"ok":true}`), 100); err != nil {
		t.Fatalf("Succeed: %v", err)
	}

	// Every transition must have an immutable event.
	eventRepo := postgres.NewEventRepo(pool)
	events, err := eventRepo.ListByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListByJobID: %v", err)
	}

	byType := make(map[string]int)
	for _, ev := range events {
		byType[string(ev.Type)]++
	}

	for _, want := range []string{"CLAIMED", "STARTED", "SUCCEEDED"} {
		if byType[want] == 0 {
			t.Errorf("missing event type %s (got event types: %v)", want, byType)
		}
	}

	// Verify terminal job state.
	if got := jobState(t, pool, job.ID); got != "SUCCEEDED" {
		t.Errorf("job state = %q; want SUCCEEDED", got)
	}
}

// TestInvariant_StaleWorkerCannotSucceedAfterRecovery proves that once the
// recovery scheduler has abandoned a job, the original worker's Succeed call
// is rejected. At-least-once semantics are preserved: the job re-enters QUEUED
// and will be re-executed rather than incorrectly marked SUCCEEDED.
func TestInvariant_StaleWorkerCannotSucceedAfterRecovery(t *testing.T) {
	pool := testhelp.OpenDB(t)
	testhelp.TruncateTables(t, pool)
	ctx := context.Background()

	w := mustCreateWorker(t, pool)
	mustCreateJob(t, pool, 3)
	job, attempt := mustClaim(t, pool, w.ID)

	leaseDuration := 30 * time.Second
	svc := lifecycle.NewService(pool, leaseDuration, zerolog.Nop())
	leaseToken := *job.LeaseToken

	// Worker starts the attempt (CLAIMED→RUNNING).
	if err := svc.Start(ctx, attempt.ID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Simulate lease expiry and recovery sweep.
	expireLease(t, pool, job.ID)
	sched := recovery.NewScheduler(pool, 10*time.Second, zerolog.Nop())
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	// Job should be QUEUED again, attempt ABANDONED.
	if got := jobState(t, pool, job.ID); got != "QUEUED" {
		t.Errorf("post-sweep job state = %q; want QUEUED", got)
	}

	// The original worker (now stale) tries to report success.
	err := svc.Succeed(ctx, attempt.ID, leaseToken, []byte(`{"ok":true}`), 100)
	if err == nil {
		t.Fatal("Succeed after recovery: expected error, got nil")
	}

	// Job must still be QUEUED (not spuriously SUCCEEDED).
	if got := jobState(t, pool, job.ID); got != "QUEUED" {
		t.Errorf("post-stale-succeed job state = %q; want QUEUED", got)
	}
}

// TestInvariant_SubmissionIdempotency proves that the submission service returns
// the same job for duplicate idempotency_key submissions, with no duplicate rows.
func TestInvariant_SubmissionIdempotency(t *testing.T) {
	pool := testhelp.OpenDB(t)
	testhelp.TruncateTables(t, pool)
	ctx := context.Background()

	jobRepo := postgres.NewJobRepo(pool)
	svc := submission.NewService(jobRepo, zerolog.Nop())

	key := "idem-" + uuid.New().String()
	req := submission.SubmitRequest{
		IdempotencyKey: key,
		Kind:           "vtest.idem",
		Payload:        []byte(`{}`),
		MaxAttempts:    3,
		TimeoutSecs:    60,
	}

	r1, err := svc.Submit(ctx, req)
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	if !r1.Created {
		t.Error("first Submit: want Created=true")
	}

	r2, err := svc.Submit(ctx, req)
	if err != nil {
		t.Fatalf("second Submit: %v", err)
	}
	if r2.Created {
		t.Error("second Submit: want Created=false (idempotent return)")
	}
	if r1.Job.ID != r2.Job.ID {
		t.Errorf("job IDs differ: first=%s second=%s (duplicate row created)", r1.Job.ID, r2.Job.ID)
	}

	// Exactly one row must exist.
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM jobs WHERE idempotency_key = $1`, key,
	).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("job row count = %d; want exactly 1", count)
	}
}

// TestInvariant_FailRequeuesWithRetryRemaining proves that lifecycle.Fail
// re-queues the job when attempt_count < max_attempts, and that the job
// becomes claimable again after the backoff window.
func TestInvariant_FailRequeuesWithRetryRemaining(t *testing.T) {
	pool := testhelp.OpenDB(t)
	testhelp.TruncateTables(t, pool)
	ctx := context.Background()

	w := mustCreateWorker(t, pool)
	mustCreateJob(t, pool, 3) // 3 attempts allowed
	job, attempt := mustClaim(t, pool, w.ID)

	leaseDuration := 30 * time.Second
	svc := lifecycle.NewService(pool, leaseDuration, zerolog.Nop())
	leaseToken := *job.LeaseToken

	if err := svc.Start(ctx, attempt.ID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := svc.Fail(ctx, attempt.ID, leaseToken, "transient error", nil); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// Job should be re-queued (not terminal).
	if got := jobState(t, pool, job.ID); got != "QUEUED" {
		t.Errorf("post-fail job state = %q; want QUEUED", got)
	}

	// REQUEUED event must exist.
	if n := countEvents(t, pool, job.ID, "REQUEUED"); n != 1 {
		t.Errorf("REQUEUED event count = %d; want 1", n)
	}

	// Reset eligible_at so a second claim works immediately (backoff is in the future).
	_, err := pool.Exec(ctx,
		`UPDATE jobs SET eligible_at = NOW() WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("reset eligible_at: %v", err)
	}

	w2 := mustCreateWorker(t, pool)
	job2, _ := mustClaim(t, pool, w2.ID)
	if job2.ID != job.ID {
		t.Errorf("second claim got job %s; want %s", job2.ID, job.ID)
	}
	if job2.AttemptCount != 2 {
		t.Errorf("attempt_count = %d; want 2", job2.AttemptCount)
	}
}

// TestInvariant_FailTerminatesWhenRetriesExhausted proves that lifecycle.Fail
// marks a job FAILED (terminal) when attempt_count >= max_attempts.
func TestInvariant_FailTerminatesWhenRetriesExhausted(t *testing.T) {
	pool := testhelp.OpenDB(t)
	testhelp.TruncateTables(t, pool)
	ctx := context.Background()

	w := mustCreateWorker(t, pool)
	mustCreateJob(t, pool, 1) // only 1 attempt allowed
	job, attempt := mustClaim(t, pool, w.ID)

	leaseDuration := 30 * time.Second
	svc := lifecycle.NewService(pool, leaseDuration, zerolog.Nop())
	leaseToken := *job.LeaseToken

	if err := svc.Start(ctx, attempt.ID, leaseToken); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Fail(ctx, attempt.ID, leaseToken, "fatal error", nil); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if got := jobState(t, pool, job.ID); got != "FAILED" {
		t.Errorf("job state = %q; want FAILED (retries exhausted)", got)
	}

	// terminal_at must be set.
	var termAt *time.Time
	pool.QueryRow(ctx, `SELECT terminal_at FROM jobs WHERE id = $1`, job.ID).Scan(&termAt) //nolint:errcheck
	if termAt == nil {
		t.Error("terminal_at is NULL; want a timestamp")
	}

	// FAILED event must exist.
	if n := countEvents(t, pool, job.ID, "FAILED"); n != 1 {
		t.Errorf("FAILED event count = %d; want 1", n)
	}
}

// TestInvariant_HeartbeatPreventsRecovery proves that lifecycle.JobHeartbeat
// extends lease_expires_at so the recovery scheduler does not reclaim a job
// from a healthy worker (the Layer 8 lease-extension fix).
func TestInvariant_HeartbeatPreventsRecovery(t *testing.T) {
	pool := testhelp.OpenDB(t)
	testhelp.TruncateTables(t, pool)
	ctx := context.Background()

	w := mustCreateWorker(t, pool)
	mustCreateJob(t, pool, 3)
	job, _ := mustClaim(t, pool, w.ID)

	leaseDuration := 30 * time.Second
	svc := lifecycle.NewService(pool, leaseDuration, zerolog.Nop())
	leaseToken := *job.LeaseToken

	// Expire the lease directly in the DB (simulates time passing).
	expireLease(t, pool, job.ID)

	// Worker sends a heartbeat — this should extend lease_expires_at by leaseDuration.
	if err := svc.JobHeartbeat(ctx, job.ID, leaseToken); err != nil {
		var stale *domain.StaleLeaseError
		if errors.As(err, &stale) {
			t.Fatalf("JobHeartbeat rejected (StaleLeaseError): the lease was already expired before heartbeat; try shortening the delay")
		}
		t.Fatalf("JobHeartbeat: %v", err)
	}

	// Now the sweep must not touch the job (lease freshly extended).
	sched := recovery.NewScheduler(pool, 10*time.Second, zerolog.Nop())
	if err := sched.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if got := jobState(t, pool, job.ID); got != "CLAIMED" {
		t.Errorf("job state = %q; want CLAIMED (heartbeat should have prevented recovery)", got)
	}
}
