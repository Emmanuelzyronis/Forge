# FORGE

Distributed job execution platform. PostgreSQL is the queue — no Redis, no RabbitMQ. Workers claim jobs atomically with `SELECT FOR UPDATE SKIP LOCKED`, hold ownership under a rotating lease token, and release back to the queue when their lease expires. A recovery scheduler detects abandoned jobs and re-enqueues them. Every state transition writes an immutable event in the same database transaction.

Built in Go as an engineering portfolio project. 13 implementation layers from scaffold to running system.

---

## Architecture

```
  Clients
     │  POST /jobs   GET /jobs/{id}
     ▼
┌──────────────────────────────────────┐
│             forge-api                │
│                                      │
│  Submission   Dispatch   Lifecycle   │
│  Service      Service    Service     │
│                                      │
│  ┌─────────────────────────────┐     │
│  │   Recovery Scheduler        │     │
│  │   (embedded goroutine,      │     │
│  │    runs every 10s)          │     │
│  └─────────────────────────────┘     │
│                                      │
│  GET /metrics   GET /healthz         │
└──────────────────────────────────────┘
         │  pgx/v5
         ▼
┌──────────────────────────────────────┐
│           PostgreSQL 16              │
│                                      │
│  jobs   job_attempts   job_events    │
│  workers                             │
└──────────────────────────────────────┘
         ▲
         │  POST /workers/{id}/claim
┌──────────────────────────────────────┐
│           forge-worker (×N)          │
│                                      │
│  Poll → Claim → Execute → Report     │
│  Heartbeat goroutine (every 10s)     │
└──────────────────────────────────────┘
```

The recovery scheduler runs inside the API process. Workers communicate with the database only through the API — no direct DB access.

---

## Key invariants

- **Exactly-one claim** — `SELECT FOR UPDATE SKIP LOCKED` ensures exactly one worker gets each job, verified by 10-goroutine concurrent test.
- **Lease-based ownership** — workers must heartbeat before `lease_expires_at`; heartbeat extends the lease. No heartbeat → lease expires → recovery re-queues.
- **At-least-once execution** — a worker crash before reporting success causes re-execution on recovery. Stated explicitly; consumers are responsible for idempotency.
- **Atomic state + event** — every state transition writes a `JobEvent` in the same database transaction as the state write (F-INV-010). Neither commits without the other.
- **Idempotent submission** — duplicate `idempotency_key` returns the existing job, no duplicate row.
- **Retry exhaustion** — when `attempt_count >= max_attempts`, the recovery scheduler terminates the job as `FAILED` rather than re-queuing.

---

## Quick start

```bash
# Start postgres, run migrations, start API and two workers
make run

# Wait ~10s for the stack to be healthy, then run the demo
make demo

# Tail logs
make logs

# Stop and remove volumes
make stop
```

Requirements: Docker with Compose v2 (`docker compose`), Python 3 (for the demo script), curl.

To run integration tests against a live database:

```bash
make run
make test-integration
```

---

## Job state machine

```
             ┌─────────────────────────────────────┐
             │              QUEUED                  │◄──────────┐
             └─────────────────────────────────────┘           │
                              │ ClaimNext                       │
                              ▼                                 │
             ┌─────────────────────────────────────┐           │ recovery
             │             CLAIMED                  │           │ re-queue
             └─────────────────────────────────────┘           │
                              │ StartAttempt                    │
                              ▼                                 │
             ┌─────────────────────────────────────┐           │
             │             RUNNING                  │───────────┘
             └─────────────────────────────────────┘
              │             │               │
              │ Succeed      │ Fail          │ lease expires
              ▼             ▼               ▼
          SUCCEEDED      FAILED          FAILED
          (terminal)   (terminal if    (terminal if
                        exhausted)      exhausted)
```

Attempt states: `CREATED → RUNNING → {SUCCEEDED, FAILED, TIMED_OUT, ABANDONED}`

---

## API reference

All requests/responses are JSON. Workers are identified by UUID assigned at registration.

### Jobs

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/jobs` | Submit a job |
| `GET` | `/jobs` | List jobs (filters: `state`, `kind`, `correlation_id`, `limit`, `offset`) |
| `GET` | `/jobs/{id}` | Get a job by ID |
| `GET` | `/jobs/{id}/attempts` | List all execution attempts for a job |
| `GET` | `/jobs/{id}/events` | List all lifecycle events for a job |
| `POST` | `/jobs/{id}/heartbeat` | Worker: extend job lease while executing |

**Submit a job:**
```bash
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "email.send",
    "payload": {"to": "user@example.com", "subject": "Hello"},
    "idempotency_key": "email-001",
    "max_attempts": 3,
    "timeout_secs": 30,
    "priority": 0
  }'
```

**List queued jobs:**
```bash
curl "http://localhost:8080/jobs?state=QUEUED&limit=10"
```

### Workers

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/workers` | Register a worker |
| `GET` | `/workers` | List active workers |
| `GET` | `/workers/{id}` | Get a worker by ID |
| `POST` | `/workers/{id}/heartbeat` | Worker: report liveness |
| `DELETE` | `/workers/{id}` | Worker: graceful offline |
| `POST` | `/workers/{id}/claim` | Worker: claim next eligible job |

### Attempt lifecycle (worker-facing)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/attempts/{id}/start` | Begin executing an attempt |
| `POST` | `/attempts/{id}/succeed` | Report successful completion |
| `POST` | `/attempts/{id}/fail` | Report failure |

### System

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check with DB probe |
| `GET` | `/metrics` | Prometheus metrics |

---

## Configuration

All settings are environment variables. Durations are in seconds.

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_DATABASE_URL` | `postgres://forge:forge@localhost:5432/forge?sslmode=disable` | PostgreSQL DSN |
| `FORGE_LISTEN_ADDR` | `:8080` | API listen address |
| `FORGE_LEASE_DURATION_SECS` | `30` | Lease duration granted on job claim |
| `FORGE_RECOVERY_INTERVAL_SECS` | `10` | How often the recovery scheduler sweeps |
| `FORGE_POLL_INTERVAL_SECS` | `1` | Worker poll interval |
| `FORGE_HEARTBEAT_INTERVAL_SECS` | `10` | Worker heartbeat interval |
| `FORGE_WORKER_NAME` | `worker-1` | Worker name (must be unique per deployment) |
| `FORGE_API_URL` | `http://localhost:8080` | API base URL (used by worker) |

---

## Prometheus metrics

The `/metrics` endpoint exposes a custom Prometheus registry (no global default pollution):

| Metric | Type | Labels |
|--------|------|--------|
| `forge_http_requests_total` | Counter | `method`, `path`, `status` |
| `forge_http_request_duration_seconds` | Histogram | `method`, `path` |
| `forge_jobs_submitted_total` | Counter | `kind` |
| `forge_jobs_claimed_total` | Counter | — |
| `forge_jobs_succeeded_total` | Counter | — |
| `forge_jobs_failed_total` | Counter | `outcome` |
| `forge_jobs_recovered_total` | Counter | `outcome` |
| `forge_job_duration_milliseconds` | Histogram | `kind` |

---

## Implementation layers

The system was built layer-by-layer, each committed independently:

| Layer | What was built |
|-------|---------------|
| 1 | Go module scaffold, config, zerolog telemetry, `/healthz`, Docker files |
| 2 | Domain model: `Job`, `ExecutionAttempt`, `Worker`, `JobEvent`, state machines, `RetryPolicy` |
| 3 | PostgreSQL schema, indexes, migration file (`001_init.up.sql`) |
| 4 | Submission service: job creation, idempotency key deduplication |
| 5 | Registration service: worker upsert (stable identity across restarts), heartbeat, offline |
| 6 | Dispatch service: atomic `ClaimNext` with `SELECT FOR UPDATE SKIP LOCKED`, lease grant |
| 7 | Lifecycle service: `Start`, `Succeed`, `Fail` with stale-lease rejection |
| 8 | Recovery scheduler: expired-lease sweep, ABANDONED event, retry/terminate decision |
| 9 | Retry policy: exponential backoff (`min(InitialDelay × 2^(n-1), MaxDelay)`, defaults 5s/30m) |
| 10 | Query API: `GET /jobs`, `GET /jobs/{id}`, attempts, events, workers |
| 11 | Observability: Prometheus metrics, HTTP logging middleware with `X-Request-ID` |
| 12 | Failure verification: 6 integration tests proving invariants against a live database |
| 13 | Product proof: Docker Compose, Makefile, demo script, this README |

---

## Crash-recovery demo

A reproducible scenario that proves the recovery path end-to-end. Postgres and the API run in Docker; workers run as local processes so the crash is visible and measurable.

```bash
make demo-crash
```

The script (`demo/scripts/demo-crash.sh`):

1. Resets the environment (`docker compose down -v`)
2. Brings up postgres + forge-api
3. Starts **worker-01** (`FORGE_WORKER_NAME=worker-01`) as a local process
4. Submits a `demo.crash-recovery` job (`max_attempts=3`, 12s execution time)
5. Waits for the job to reach **RUNNING**
6. Sends **SIGKILL** to worker-01 — no sentinel write
7. Waits ~40s for the recovery scheduler to detect the expired lease and re-queue
8. Starts **worker-02** (`FORGE_WORKER_NAME=worker-02`)
9. Waits for the job to **SUCCEED** on attempt 2
10. Collects evidence → `demo/evidence/raw-evidence.json` + `demo/evidence/presentation.json`

See [docs/DEMO.md](docs/DEMO.md) for full documentation.

### Rendering the portfolio video

```bash
make demo-video
# Output: remotion/out/forge-demo.mp4  (1920×1080 60fps ~90s)
```

The Remotion project (`remotion/`) consumes `demo/evidence/presentation.json` (or the fixture at `demo/fixtures/worker-crash-recovery.json` if no live run has been done) and renders 7 scenes: Problem → Normal Execution → Worker Failure → Lease Expiration → Recovery → Success → Evidence.

---

## Repository

[github.com/Emmanuelzyronis/Forge](https://github.com/Emmanuelzyronis/Forge)
