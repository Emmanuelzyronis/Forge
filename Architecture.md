# FORGE Architecture v1.0

**Project:** FORGE
**Document:** Architecture Specification — Discovery Phase
**Version:** 1.0
**Status:** Specification complete. Ready for implementation review.
**Primary concern:** Reliable asynchronous job execution under distributed failure conditions.

---

## 1. Architectural Identity

> **FORGE is a distributed job execution platform with explicitly defined semantics for job state, execution ownership, lease-based concurrency, retry policy, failure recovery, and execution evidence.**

FORGE is designed to accept asynchronous work, dispatch it to workers under exclusive ownership, detect and recover from worker failure, enforce retry bounds, and preserve a complete, auditable execution history for every job.

FORGE is not a workflow engine, a scheduler for cron tasks, a general compute platform, or a wrapper over an existing queue library.

The engineering objective is to demonstrate deliberate reasoning about: what happens when workers, queues, processes, networks, and execution attempts fail independently, and how a system can be designed to remain correct across those failure modes.

---

## 2. Relationship to LEDGER

LEDGER demonstrates: deterministic decision systems, reconciliation, canonical data, immutable evidence, auditability, correctness under data ambiguity.

FORGE demonstrates: distributed execution, asynchronous processing, concurrency, worker coordination, queue semantics, execution ownership, retries, failure recovery, idempotency, observability, operational correctness.

The two projects are architecturally independent.

FORGE reuses engineering principles:
- Explicit contracts
- Authoritative source-of-truth ownership
- Immutable evidence where appropriate
- Deterministic state transitions
- Failure verification as a first-class concern
- Structured telemetry with correlation identifiers
- Evidence-driven product completion

FORGE does not copy LEDGER's implementation. In particular:
- Language: Go, not Python
- Database: PostgreSQL, not SQLite
- Processing model: distributed workers with lease ownership, not a sequential batch pipeline
- Queue: database-backed claim semantics, not a file-based pipeline runner
- Deployment: Docker Compose multi-process, not Railway single-process

---

## A. Product Definition

### A.1 One-line definition

FORGE is a distributed job execution platform that submits, dispatches, tracks, retries, and recovers asynchronous work under explicit ownership semantics.

### A.2 Mission

Build a job execution system that can accept asynchronous work, make it eligible for execution, dispatch it to workers under exclusive lease-based ownership, detect and recover from worker failure, enforce bounded retry policy, and preserve sufficient evidence to explain the complete lifecycle of every job.

### A.3 Users

For portfolio purposes, FORGE has one class of user: an engineer or automated system submitting jobs and expecting durable, eventually successful execution with full visibility into what happened.

### A.4 Primary use cases

1. Submit a unit of work and receive a durable job identity.
2. Workers claim and execute jobs under exclusive ownership.
3. Detect worker failure and recover abandoned jobs.
4. Enforce bounded retry policy.
5. Inspect the complete execution history of any job.
6. Observe system state: queue depth, active workers, failure rates.

### A.5 Core workflows

**Submission:** Client submits a job → FORGE persists it → job becomes eligible for dispatch → client receives durable job ID.

**Execution:** Worker polls for eligible jobs → claims one atomically → executes → reports outcome → job reaches terminal state.

**Recovery:** Worker disappears → lease expires → recovery scheduler detects expiry → creates retry attempt → job returns to queue.

**Inspection:** Client queries job → receives complete state, all attempt history, all events.

### A.6 Scope

- Job submission with idempotency
- Persistent job identity
- Worker registration and heartbeat
- Lease-based exclusive claim
- Execution attempt tracking
- Bounded retry policy
- Timeout enforcement
- Recovery from worker failure
- Full execution history
- Structured observability (logs, metrics)
- REST API for submission, inspection, and worker protocol
- CLI/script demonstration of all failure scenarios

### A.7 Non-goals (v1.0)

- Cron or calendar scheduling
- DAG workflows or job dependencies
- Priority queues (extensible but not implemented)
- Multi-tenant isolation
- Role-based access control
- Kubernetes deployment
- Distributed tracing (OpenTelemetry)
- Dead-letter queue UI
- Job cancellation (defined semantics only; not implemented in v1.0)
- Plugin or arbitrary user code execution
- Multi-region or global consensus
- Autoscaling workers

### A.8 Success criteria

1. A submitted job can be traced from submission to terminal outcome through its complete attempt history.
2. Worker crash is detected within one lease interval and recovery begins automatically.
3. Concurrent workers cannot simultaneously hold a valid active claim on the same job.
4. Duplicate submissions with the same idempotency key do not create unintended duplicate jobs.
5. Retry exhaustion produces a deterministic terminal failure.
6. The system recovers correctly after process restart with no state loss.
7. Every failure scenario in the product proof produces machine-verifiable evidence.

---

## B. Requirements

### B.1 Functional requirements

**FR-001 — Job submission**
The system shall accept job submissions containing a job type, payload, optional idempotency key, optional retry policy, and optional timeout. It shall return a durable job identity.

**FR-002 — Durable persistence**
Every accepted job and every execution attempt shall be persisted durably before control is returned to the submitter or worker.

**FR-003 — Idempotent submission**
A submission with the same idempotency key shall return the existing job identity rather than creating a duplicate job.

**FR-004 — Worker registration**
Workers shall register with the system and receive a durable worker identity. Registration shall be idempotent on restart with the same logical worker identity.

**FR-005 — Exclusive claim**
A worker shall claim a queued job through an atomic operation that prevents any other worker from concurrently claiming the same job.

**FR-006 — Lease-based ownership**
A claimed job shall carry a lease with an expiry timestamp. The worker must renew the lease before expiry to maintain ownership. An expired lease is invalid.

**FR-007 — Attempt tracking**
Each execution attempt shall be an independent, persistent record. Multiple attempts for the same job shall be preserved without overwriting prior attempts.

**FR-008 — Retry policy**
The system shall enforce a configured maximum attempt count. When an attempt fails and retries remain, the system shall create a new attempt and make the job eligible again. When retries are exhausted, the job shall reach a terminal failed state.

**FR-009 — Timeout enforcement**
If an execution attempt exceeds its configured timeout, the system shall treat it as a failed attempt subject to retry policy.

**FR-010 — Recovery**
The recovery scheduler shall periodically detect leases that have expired without completion, mark the corresponding attempt as abandoned, and initiate recovery according to retry policy.

**FR-011 — Execution history**
Any job shall be inspectable for its complete execution history: all attempts, attempt states, attempt durations, worker identities, failure reasons, and events.

**FR-012 — System observability**
The system shall expose queue depth, active jobs, worker states, failure counts, recovery counts, and execution latency through a metrics endpoint and structured logs.

### B.2 Non-functional requirements

**NFR-001 — Local reproducibility**
The complete system shall be reproducible locally via Docker Compose. No external service account required.

**NFR-002 — Crash safety**
Every authoritative state write shall be transactional. A process crash at any point shall leave the system in a recoverable state with no phantom commits.

**NFR-003 — Recovery latency**
A failed worker lease shall be detected within two lease expiry intervals (configurable; default: 60 seconds per interval, detection within 120 seconds).

**NFR-004 — Observability completeness**
Every state transition shall emit a structured log line with `job_id`, `attempt_id`, `worker_id`, `from_state`, `to_state`, and timestamp.

**NFR-005 — Claim exclusivity**
Under concurrent load with multiple workers, no job shall be simultaneously claimed by more than one worker.

### B.3 Failure requirements

**FAIL-001** — A worker that claims a job and crashes shall not leave the job permanently stuck. The job shall be recoverable within the detection window.

**FAIL-002** — A worker that successfully completes work but crashes before reporting shall result in a retry of the attempt (at-least-once execution semantics; explicitly not exactly-once).

**FAIL-003** — A stale worker that attempts to complete a job after its lease has been transferred to another worker shall be rejected.

**FAIL-004** — A process restart of the API server shall not lose any accepted job or attempt.

**FAIL-005** — A duplicate submission (same idempotency key submitted twice concurrently or sequentially) shall not create a duplicate job.

**FAIL-006** — When retry count is exhausted, the job shall reach a terminal failed state and not be retried again.

**FAIL-007** — The recovery scheduler's crash shall not cause job loss; jobs with expired leases shall be detected on next scheduler run.

### B.4 Security requirements

**SEC-001** — No secrets in source code or repository.
**SEC-002** — Worker-to-API communication is authenticated by a shared secret (bearer token) in v1.0.
**SEC-003** — Payloads stored in PostgreSQL with no logging of payload content.
**SEC-004** — Database credentials stored in environment variables only.

### B.5 Observability requirements

**OBS-001** — Every job state transition emits a structured JSON log line.
**OBS-002** — Every attempt lifecycle event emits a structured log line.
**OBS-003** — Prometheus metrics endpoint at `/metrics` exposing: submitted jobs total, queued jobs gauge, running jobs gauge, succeeded jobs total, failed jobs total, retried attempts total, abandoned attempts total, recovery operations total, execution duration histogram, queue wait duration histogram, lease renewal count.
**OBS-004** — Worker heartbeat gaps emitting a log line when renewal is late.
**OBS-005** — `/healthz` endpoint returning database connectivity status and scheduler liveness.

### B.6 Performance requirements

Performance is not the primary objective of FORGE v1.0. The system should support the demonstration scenarios without becoming a bottleneck at single-digit worker count and tens of jobs per second. Numerical SLOs are deferred until a representative benchmark is measured in Layer 13.

---

## C. Architecture

### C.1 Component architecture

```text
┌─────────────────────────────────────────────────────────────────┐
│                        FORGE System                             │
│                                                                 │
│  ┌─────────────┐     ┌─────────────────────────────────────┐   │
│  │  Client     │────▶│           FORGE API Server          │   │
│  │  (curl/CLI) │     │  ┌──────────┐  ┌─────────────────┐  │   │
│  └─────────────┘     │  │Submission│  │  Query/Inspect  │  │   │
│                       │  │ Service  │  │    Service      │  │   │
│                       │  └────┬─────┘  └────────────────┘  │   │
│                       │       │                              │   │
│                       │  ┌────▼──────────────────────────┐  │   │
│                       │  │       Job Service             │  │   │
│                       │  │  (state machine enforcement)  │  │   │
│                       │  └────┬──────────────────────────┘  │   │
│                       └───────┼──────────────────────────────┘  │
│                               │                                  │
│                       ┌───────▼──────────────────────────────┐  │
│                       │        PostgreSQL                     │  │
│                       │   ┌──────────┐  ┌─────────────────┐  │  │
│                       │   │  jobs    │  │execution_attempts│  │  │
│                       │   │ (queue)  │  └─────────────────┘  │  │
│                       │   └──────────┘  ┌─────────────────┐  │  │
│                       │   ┌──────────┐  │  job_events     │  │  │
│                       │   │ workers  │  │  (audit trail)  │  │  │
│                       │   └──────────┘  └─────────────────┘  │  │
│                       └──────────────────────────────────────┘  │
│                               ▲                                  │
│            ┌──────────────────┤──────────────────────┐          │
│            │                  │                      │          │
│    ┌───────┴──────┐   ┌───────┴──────┐   ┌──────────┴───────┐  │
│    │   Worker A   │   │   Worker B   │   │    Scheduler     │  │
│    │  (goroutine  │   │  (goroutine  │   │  (background     │  │
│    │   or proc)   │   │   or proc)   │   │   goroutine)     │  │
│    └──────────────┘   └──────────────┘   └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### C.2 Data flow — normal execution

```text
Client
  │  POST /v1/jobs
  ▼
API Handler
  │  validate input
  │  check idempotency_key
  │  insert job (state=QUEUED)
  │  insert job_event (SUBMITTED)
  │  return job_id
  ▼
PostgreSQL jobs table (state=QUEUED)
  │
  │  (worker polls: SELECT FOR UPDATE SKIP LOCKED)
  ▼
Worker
  │  acquire lease atomically
  │  update job state → CLAIMED
  │  insert attempt (state=CREATED)
  │  update attempt → RUNNING
  │  heartbeat loop (renew lease)
  │  execute work
  │  update attempt → SUCCEEDED
  │  update job → SUCCEEDED
  │  insert job_event (SUCCEEDED)
  ▼
PostgreSQL jobs table (state=SUCCEEDED)
```

### C.3 Data flow — worker failure and recovery

```text
Worker A
  │  claims job (state=CLAIMED)
  │  starts execution (state=RUNNING)
  │
  X  CRASH (no heartbeat)
  │
  │  lease expires at lease_expires_at
  ▼
Scheduler (recovery sweep)
  │  SELECT jobs WHERE state IN (CLAIMED, RUNNING)
  │       AND lease_expires_at < NOW()
  │       FOR UPDATE SKIP LOCKED
  │  mark attempt → ABANDONED
  │  increment job.attempt_count
  │  if attempt_count < max_attempts:
  │    create new attempt
  │    job → QUEUED
  │  else:
  │    job → FAILED
  │  emit job_events
  ▼
Worker B
  │  polls queue
  │  claims job (new lease, new lease_token)
  │  executes
  │  succeeds
  ▼
PostgreSQL jobs table (state=SUCCEEDED)
```

### C.4 Control flow — claim protocol

```text
Worker.poll():
  LOOP:
    tx = BEGIN
    row = SELECT FROM jobs
          WHERE state = 'QUEUED' AND eligible_at <= NOW()
          ORDER BY priority DESC, queued_at ASC
          LIMIT 1
          FOR UPDATE SKIP LOCKED
    IF no row: COMMIT; sleep(poll_interval); continue

    attempt_id = new_uuid()
    lease_token = new_uuid()
    lease_expires_at = NOW() + lease_duration

    INSERT INTO execution_attempts (attempt_id, job_id, ...)
    UPDATE jobs SET state='CLAIMED', current_attempt_id=attempt_id,
                    lease_token=lease_token, lease_expires_at=lease_expires_at
    INSERT INTO job_events (...)
    COMMIT
    → return (job, attempt_id, lease_token)
```

### C.5 Control flow — recovery sweep

```text
Scheduler.recoverStaleClaims():
  LOOP every recovery_interval:
    tx = BEGIN
    rows = SELECT job_id, attempt_id FROM jobs
           WHERE state IN ('CLAIMED','RUNNING')
             AND lease_expires_at < NOW()
           FOR UPDATE SKIP LOCKED
           LIMIT 100

    FOR EACH row:
      UPDATE execution_attempts SET state='ABANDONED', completed_at=NOW()
      job.attempt_count += 1
      IF job.attempt_count < job.max_attempts:
        INSERT new execution_attempt
        UPDATE job state='QUEUED', eligible_at=NOW()+backoff
      ELSE:
        UPDATE job state='FAILED', terminal_at=NOW()
      INSERT job_event
    COMMIT
```

### C.6 Deployment architecture

```text
Docker Compose:
  ┌───────────────┐
  │  postgres:16  │  authoritative store + queue
  │  (port 5432)  │
  └───────┬───────┘
          │
  ┌───────┴───────┐
  │ forge-api     │  REST API + embedded scheduler
  │ (port 8080)   │
  │ /healthz      │
  │ /metrics      │
  └───────┬───────┘
          │
  ┌───────┴───────┐
  │ forge-worker  │  1 or more instances
  │ (--count=2)   │
  └───────────────┘
```

Workers and API connect to the same PostgreSQL instance. Multiple worker instances may run concurrently as separate processes or goroutines. The scheduler is embedded in the API process (goroutine) but can be extracted to a separate binary.

### C.7 Persistence model

See Domain Model (Section D) for entity definitions.

**Authoritative:**
- `jobs` — source of truth for job lifecycle state
- `execution_attempts` — source of truth for each execution attempt
- `workers` — source of truth for registered workers
- `job_events` — append-only audit trail; never updated in place
- `idempotency_keys` — submission deduplication

**Derived:**
- System metrics (Prometheus counters/gauges, rebuilt from events)
- Dashboard statistics (rebuilt from authoritative tables)

**Ephemeral:**
- Worker in-memory state (rebuilt on reconnect)
- Queue poll cache (rebuilt on next poll)

Nothing authoritative ever passes through memory-only state.

### C.8 Queue and broker model

FORGE uses **PostgreSQL as the queue** via `SELECT ... FOR UPDATE SKIP LOCKED`.

This is a deliberate architectural decision, not a simplification. Reasons:

1. **Eliminates the database-broker split problem.** When a separate message broker is used, a database commit followed by broker publish failure creates a window where the job is persisted but never dispatched. Recovery requires an outbox pattern. With the database as queue, there is no split — the commit that persists the job IS the enqueue operation.

2. **`SKIP LOCKED` provides exactly the concurrent claim semantics needed.** Multiple workers can poll simultaneously; each gets a different row; no coordination is required at the application layer.

3. **Recovery is a query.** Jobs with expired leases are simply visible in a recovery query. No dead-letter queue required. No broker-side redelivery logic required.

4. **Authoritative state is the queue state.** At any moment, a query against the jobs table answers "what is eligible to run?" definitively.

**Trade-off explicitly accepted:** At very high throughput (thousands of jobs/second), a dedicated message broker would outperform database polling. This is acceptable for FORGE's purpose. The architecture documents this boundary explicitly.

**What happens if the database is unavailable?**
Workers cannot claim jobs. They retry connection on a backoff schedule. No jobs are dispatched during database unavailability. When the database recovers, normal operation resumes. Jobs with expired leases are recovered on the next scheduler sweep.

**Optional fast-path (not in v1.0):** A Redis pub/sub signal can notify idle workers immediately when a job is enqueued rather than waiting for the next poll interval. This is an optimization only; Redis is never authoritative. If Redis is unavailable, polling still works.

### C.9 Worker model

Workers are **separate OS processes** (Go binaries) that:

1. Register with the API to receive a `worker_id`.
2. Poll PostgreSQL for eligible jobs via `SELECT FOR UPDATE SKIP LOCKED`.
3. Execute the claimed job in-process (simulated work for demonstration purposes).
4. Renew the lease via heartbeat during execution.
5. Report success or failure to the API.

Workers are separate processes (not goroutines in the API process) so that:
- Individual workers can be killed with `kill -9` or `docker kill` to demonstrate crash recovery.
- Worker failure does not affect the API or scheduler.
- Worker identity survives crash and restart with the same worker name.

### C.10 Ownership and lease model

A lease is not a separate entity — it is a set of fields on the job record:

```
jobs.current_attempt_id   UUID of the currently executing attempt
jobs.lease_token          UUID rotated on every new claim or recovery
jobs.lease_expires_at     TIMESTAMPTZ: when this lease expires
jobs.last_heartbeat_at    TIMESTAMPTZ: last received heartbeat
```

**Ownership acquisition:** `SELECT FOR UPDATE SKIP LOCKED` + atomic update sets `lease_token`, `lease_expires_at`, `current_attempt_id`. This is one transaction.

**Ownership verification:** Every attempt completion, failure report, or heartbeat must supply the `lease_token`. The API validates that the supplied token matches `jobs.lease_token`. If they do not match, the operation is rejected with a stale-lease error.

**Ownership expiry:** The scheduler detects rows where `lease_expires_at < NOW()` and `state IN (CLAIMED, RUNNING)`. It marks the attempt ABANDONED, rotates the `lease_token` (invaliding the stale worker's token), and either requeues or terminates the job.

**Why `lease_token` rotation matters:** When a lease expires and recovery creates a new attempt, the stale worker may still be alive and attempt to report success. Without token rotation, its stale success report would be accepted, marking a job succeeded that the new attempt is currently executing. With token rotation, the stale worker's request is rejected (token mismatch).

### C.11 Failure and recovery model

For each critical operation:

| Operation | Failure point | Committed state | Uncommitted state | Recovery |
|---|---|---|---|---|
| Job submission | After DB commit, before response | Job persisted | Client sees timeout | Client retries with same idempotency_key; returns existing job |
| Job submission | DB failure | Nothing | Rolled back | Client retries; operation repeatable |
| Worker claim | Crash after SELECT, before UPDATE | Nothing committed | Lock released | Job remains QUEUED; other workers pick it up |
| Worker claim | Crash after UPDATE | Job CLAIMED with lease | — | Lease expires; recovery detects and requeues |
| Heartbeat | Network failure | Lease not renewed | — | If no renewal arrives, lease expires; recovery kicks in |
| Work completion | Worker succeeds but crashes before reporting | Attempt still RUNNING | External side effect occurred | Lease expires; retry created; work may be re-executed (at-least-once) |
| Work completion | DB failure on terminal write | Attempt still RUNNING | Rolled back | Worker retries completion; idempotent via attempt state check |
| Recovery sweep | Scheduler crashes mid-sweep | Some jobs recovered in committed tx | Some not yet processed | Next scheduler run recovers remaining jobs |
| Recovery sweep | DB failure | Rolled back | — | Scheduler retries on next interval |

**Execution semantics:** FORGE provides **at-least-once execution semantics**. A job may be executed more than once if the worker completes work but fails to report before the lease expires. This is explicitly stated and not hidden. Job implementations should be designed with this in mind (idempotent at the business logic layer). Exactly-once state transitions (job state in PostgreSQL) are guaranteed via atomic transactions.

### C.12 Concurrency model

**Two workers claiming same job:**
Prevented by `SELECT FOR UPDATE SKIP LOCKED`. One worker gets the row lock; the other's SKIP LOCKED causes it to skip the row and get the next eligible job. This is the primary correctness mechanism and must be verified by V4 tests with concurrent workers.

**Retry racing with lease renewal:**
The `lease_token` prevents this. Recovery rotates the token; the stale worker's renewal attempt fails token validation and is rejected.

**Timeout racing with successful completion:**
The API validates state before accepting a completion report. If the job is already TIMED_OUT (recovery has occurred), the completion is rejected. The worker receives a stale-lease error.

**Duplicate submission racing with creation:**
The `idempotency_key` unique constraint in PostgreSQL serializes concurrent duplicate submissions. One INSERT succeeds; the other receives a unique constraint violation and the handler returns the existing record.

**Concurrent recovery and active worker:**
Both the recovery scheduler and the worker interact with the same row. `SELECT FOR UPDATE SKIP LOCKED` on the recovery sweep means: if the worker is in the middle of a transaction (e.g., writing its completion), the recovery sweep skips that row. If the worker is not in a transaction, and the lease has expired, recovery proceeds.

---

## D. Domain Model

### D.1 Job

The central entity. Represents a unit of work from submission to terminal outcome.

```
Job {
  job_id           UUID        — durable identity, immutable
  idempotency_key  TEXT        — submission deduplication; nullable; unique
  job_type         TEXT        — categorizes work; determines executor
  payload          JSONB       — input data; immutable after submission
  state            JobState    — current lifecycle state
  priority         INTEGER     — dispatch priority; default 0; higher = sooner
  max_attempts     INTEGER     — retry limit; default 3
  attempt_count    INTEGER     — number of attempts created so far
  timeout_secs     INTEGER     — per-attempt execution deadline; default 300
  created_at       TIMESTAMPTZ — submission timestamp; immutable
  queued_at        TIMESTAMPTZ — when job became eligible; set on creation and re-queue
  eligible_at      TIMESTAMPTZ — earliest dispatch time (allows retry backoff)
  terminal_at      TIMESTAMPTZ — when job reached a terminal state; nullable
  current_attempt_id UUID      — active attempt; nullable
  lease_token      UUID        — current lease identity; rotated on each new claim
  lease_expires_at TIMESTAMPTZ — when active lease expires; nullable
  last_heartbeat_at TIMESTAMPTZ — last heartbeat received; nullable
  correlation_id   TEXT        — caller-provided correlation; nullable
}
```

**Immutable fields:** `job_id`, `idempotency_key`, `job_type`, `payload`, `created_at`.
**Mutable by state machine only:** `state`, `attempt_count`, `terminal_at`, `current_attempt_id`, `lease_token`, `lease_expires_at`, `last_heartbeat_at`, `queued_at`, `eligible_at`.

### D.2 ExecutionAttempt

An independent record of one execution attempt. Never overwritten.

```
ExecutionAttempt {
  attempt_id       UUID          — immutable identity
  job_id           UUID          — parent job; FK
  attempt_number   INTEGER       — 1-indexed sequence per job
  worker_id        UUID          — executing worker; nullable (set on claim)
  state            AttemptState  — attempt lifecycle state
  lease_token      UUID          — lease token at claim time; immutable after claim
  started_at       TIMESTAMPTZ   — when worker confirmed execution start; nullable
  completed_at     TIMESTAMPTZ   — when attempt reached terminal state; nullable
  failure_reason   TEXT          — short failure category; nullable
  failure_detail   JSONB         — extended failure context; nullable
  result           JSONB         — success result; nullable
  duration_ms      INTEGER       — execution duration; nullable
  created_at       TIMESTAMPTZ   — when attempt was created
}
```

**Immutable after terminal:** all fields frozen when state is SUCCEEDED, FAILED, TIMED_OUT, or ABANDONED.
**Never deleted:** attempts are append-only. Attempt N is never removed when attempt N+1 is created.

### D.3 Worker

A registered execution agent.

```
Worker {
  worker_id        UUID          — identity; generated at registration
  worker_name      TEXT          — human-readable name; unique per deployment
  hostname         TEXT          — host the worker process runs on
  pid              INTEGER       — OS process ID; informational
  state            WorkerState   — current worker lifecycle state
  capabilities     TEXT[]        — job types this worker can execute; empty = all
  registered_at    TIMESTAMPTZ   — first registration timestamp
  last_heartbeat_at TIMESTAMPTZ  — last heartbeat from this worker
  last_job_id      UUID          — most recently executed job; nullable
}
```

### D.4 JobEvent (audit)

An immutable record of a state transition or significant event.

```
JobEvent {
  event_id         UUID        — immutable identity
  job_id           UUID        — subject job
  attempt_id       UUID        — related attempt; nullable
  worker_id        UUID        — related worker; nullable
  event_type       EventType   — what happened
  from_state       TEXT        — state before transition; nullable
  to_state         TEXT        — state after transition
  metadata         JSONB       — additional context
  occurred_at      TIMESTAMPTZ — when event occurred; immutable
  correlation_id   TEXT        — propagated from job
}
```

**Append-only.** Events are never updated or deleted.

### D.5 IdempotencyKey

Submission deduplication record.

```
IdempotencyKey {
  key              TEXT        — the idempotency key; PK
  job_id           UUID        — associated job; immutable
  created_at       TIMESTAMPTZ — when key was recorded
}
```

### D.6 RetryPolicy

Not a separate entity — encoded as fields on Job.

```
RetryPolicy {
  max_attempts     INTEGER     — maximum total attempts; minimum 1
  backoff_secs     INTEGER     — delay before next attempt after failure; default 0
  backoff_multiplier FLOAT     — optional exponential backoff; default 1.0 (linear)
}
```

### D.7 Entity relationships

```text
IdempotencyKey ──(1:1)──▶ Job ──(1:N)──▶ ExecutionAttempt
                           │
                           └──(1:N)──▶ JobEvent ◀──── ExecutionAttempt
                                                 ◀──── Worker
                           │
                           └──(N:1)──▶ Worker (current attempt owner)
```

---

## E. State Machines

### E.1 Job lifecycle

```text
                     ┌─────────────┐
                     │   QUEUED    │◀─────────────────────────────┐
                     └──────┬──────┘                              │
                            │  worker claims (atomic SKIP LOCKED) │
                            ▼                                      │
                     ┌─────────────┐                              │
                     │   CLAIMED   │                              │
                     └──────┬──────┘                              │
                            │  worker confirms start              │
                            ▼                                      │
                     ┌─────────────┐       attempt failed,        │
                     │   RUNNING   │───── retries remain ─────────┘
                     └──────┬──────┘
              ┌─────────────┼─────────────┐
              ▼             ▼             ▼
       ┌──────────┐  ┌──────────┐  ┌──────────┐
       │SUCCEEDED │  │  FAILED  │  │TIMED_OUT │
       │(terminal)│  │(terminal)│  │(terminal)│
       └──────────┘  └──────────┘  └──────────┘
```

**State definitions:**

| State | Meaning | Terminal |
|---|---|---|
| QUEUED | Eligible for dispatch; no active owner | No |
| CLAIMED | Worker holds lease; execution not yet confirmed | No |
| RUNNING | Worker is actively executing; heartbeat expected | No |
| SUCCEEDED | Execution completed successfully | Yes |
| FAILED | Retry exhaustion or non-retryable failure | Yes |
| TIMED_OUT | Attempt exceeded timeout after retry exhaustion | Yes |

**Legal transitions:**

| From | To | Trigger | Atomicity requirement |
|---|---|---|---|
| QUEUED | CLAIMED | Worker claim (SKIP LOCKED) | Atomic claim + lease write |
| CLAIMED | RUNNING | Worker start signal | Atomic attempt update + event |
| CLAIMED | QUEUED | Lease expiry (recovery) | Atomic abandon + requeue + event |
| RUNNING | SUCCEEDED | Worker success report | Atomic attempt + job + event |
| RUNNING | QUEUED | Lease expiry, retries remain | Atomic abandon + requeue + event |
| RUNNING | FAILED | Lease expiry, retries exhausted; or worker non-retryable failure | Atomic abandon/fail + job + event |
| RUNNING | TIMED_OUT | Execution exceeds timeout, retries exhausted | Atomic timeout + job + event |

**Illegal transitions (enforced by service layer):**
- SUCCEEDED → any state
- FAILED → any state
- TIMED_OUT → any state
- QUEUED → RUNNING (must pass through CLAIMED)

### E.2 ExecutionAttempt lifecycle

```text
                  ┌──────────┐
                  │ CREATED  │
                  └─────┬────┘
                        │  worker confirms execution start
                        ▼
                  ┌──────────┐
                  │ RUNNING  │
                  └─────┬────┘
          ┌─────────────┼─────────────┬─────────────┐
          ▼             ▼             ▼             ▼
   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
   │SUCCEEDED │  │  FAILED  │  │TIMED_OUT │  │ABANDONED │
   │(terminal)│  │(terminal)│  │(terminal)│  │(terminal)│
   └──────────┘  └──────────┘  └──────────┘  └──────────┘
```

**ABANDONED** is the attempt state when the lease expires and recovery creates a new attempt. It is distinct from FAILED (worker reported failure) to preserve the distinction between "worker disappeared" and "worker explicitly failed."

### E.3 Worker lifecycle

```text
           ┌──────────────┐
           │  REGISTERED  │
           └──────┬───────┘
                  │  first heartbeat
                  ▼
           ┌──────────────┐       ◀──── claim job
           │     IDLE     │ ─────────────────────────┐
           └──────┬───────┘                          │
                  │                                  ▼
                  │                          ┌──────────────┐
                  │                          │     BUSY     │
                  │  heartbeat timeout       └──────┬───────┘
                  ▼                                 │ job completes
           ┌──────────────┐   ◀────────────────────┘
           │    STALE     │
           └──────┬───────┘
                  │  extended timeout or explicit disconnect
                  ▼
           ┌──────────────┐
           │   OFFLINE    │
           └──────────────┘
```

Workers can re-register after disconnect. Re-registration with the same `worker_name` reactivates the worker record (rather than creating a duplicate). On re-registration, the `pid` and `registered_at` are updated.

### E.4 Lease lifecycle

```text
           ┌─────────────┐
           │  NO LEASE   │  (job QUEUED state)
           └──────┬──────┘
                  │  atomic claim
                  ▼
           ┌─────────────┐
           │   ACTIVE    │  (lease_expires_at in future)
           └──────┬──────┘
          ┌───────┴───────┐
          │               │
          ▼               ▼
   ┌─────────────┐  ┌─────────────┐
   │  RENEWED    │  │   EXPIRED   │
   │ (heartbeat) │  │ (no renewal)│
   └──────┬──────┘  └──────┬──────┘
          │                │  recovery sweep detects
          └──────────┐     ▼
                     │  ┌─────────────┐
                     │  │  ABANDONED  │  (attempt state)
                     │  └─────────────┘
                     │
                     ▼
              terminal on job completion
```

**Lease invariant:** At most one ACTIVE lease exists per job at any time. This is enforced by `SELECT FOR UPDATE SKIP LOCKED` on claim and by `lease_token` validation on all subsequent operations.

### E.5 Retry lifecycle

```text
                  Job submitted
                       │
              max_attempts = N (e.g., 3)
                       │
              attempt_count = 0 (QUEUED)
                       │
               ┌───────▼───────┐
               │  Attempt 1    │
               │  (ABANDONED)  │
               └───────┬───────┘
              attempt_count = 1
                       │ retry
               ┌───────▼───────┐
               │  Attempt 2    │
               │  (FAILED)     │
               └───────┬───────┘
              attempt_count = 2
                       │ retry
               ┌───────▼───────┐
               │  Attempt 3    │
               │  (SUCCEEDED)  │
               └───────────────┘
              job → SUCCEEDED
              attempt_count = 3 = max_attempts
```

When `attempt_count >= max_attempts` and the current attempt fails or is abandoned, the job transitions to FAILED (or TIMED_OUT) rather than QUEUED. Retry exhaustion is a deterministic terminal condition.

---

## F. Technology Decision Record

### F-TDR-001 — Language and runtime: Go

**Requirement:** Concurrency for multiple workers; process isolation for crash demonstration; networking for API and worker-API communication; fast startup; reliability; portfolio differentiation from LEDGER (Python).

**Candidates:**
1. Python (LEDGER's language; asyncio; multiprocessing)
2. Go (goroutines; compiled binary; context-based cancellation)
3. Rust (extreme correctness; high complexity)
4. Java/Kotlin (JVM ecosystem; heavyweight)
5. Node.js (async-first; single-threaded model)

**Evaluation criteria:**
- Concurrency model for worker processes
- Worker crash demonstrability (process kills)
- Existing LEDGER stack (avoid duplication)
- Portfolio signal (breadth)
- Development speed
- Operational clarity

**Chosen: Go**

**Why:**
- Goroutines are the natural model for concurrent workers executing jobs simultaneously.
- `context.Context` is the idiomatic mechanism for lease timeouts, heartbeat deadlines, and cancellation — directly maps to FORGE's domain concepts.
- Compiled binary: workers can be separate OS processes, individually killable via `kill -9` or `docker kill` for crash demonstration.
- No GIL: true parallelism for concurrent workers within a process.
- Standard library: `net/http`, `database/sql`, `sync`, `time` are all that's needed without heavy framework dependencies.
- Explicit error handling forces deliberate failure reasoning.
- Different from LEDGER: portfolio demonstrates breadth across Python (reconciliation) and Go (distributed execution).

**Trade-offs:**
- More verbose than Python for data transformation.
- Less mature ORM ecosystem (using `sqlx` or raw `database/sql`).
- Goroutine leak debugging requires care.
- Engineer unfamiliar with Go must learn it.

**Rejected:**
- Python: GIL prevents true parallelism; already LEDGER's language; multiprocessing is clunkier for inter-process coordination.
- Rust: steep learning curve would slow the project; no portfolio benefit that Go doesn't also provide.
- Java: heavyweight; JVM startup time; no meaningful advantage here.
- Node.js: single-threaded event loop model is awkward for CPU-bound worker simulation; process management less natural.

**Operational consequences:**
- Binary deployment; Docker image is small (scratch or alpine + binary).
- No Python virtual environments to manage.
- `go test ./...` for all tests; `go vet` + `golangci-lint` for static analysis.

---

### F-TDR-002 — Database: PostgreSQL

**Requirement:** Concurrent writes from multiple workers; atomic state transitions; lease management; job queue semantics; append-only audit trail; portfolio differentiation from LEDGER (SQLite).

**Candidates:**
1. SQLite (LEDGER's database; embedded; single writer)
2. PostgreSQL (relational; MVCC; row-level locking; SKIP LOCKED)
3. MySQL/MariaDB (relational; InnoDB locking)
4. MongoDB (document store; no native SKIP LOCKED)
5. Redis (in-memory; not authoritative)

**Evaluation criteria:**
- Concurrent write performance with multiple workers
- `SELECT FOR UPDATE SKIP LOCKED` support (critical for queue semantics)
- ACID transaction support for atomic state + audit writes
- Row-level locking granularity
- Local reproducibility (Docker)
- Portfolio differentiation

**Chosen: PostgreSQL (v16)**

**Why:**
- `SELECT ... FOR UPDATE SKIP LOCKED` is the exact primitive needed for concurrent exclusive job claim. Workers can poll simultaneously; each gets a different row; no application-level coordination.
- Row-level locking: lease updates, recovery sweeps, and worker completions all target specific rows without blocking unrelated jobs.
- `TIMESTAMPTZ` with full timezone support for lease expiry timestamps.
- `JSONB` for flexible job payload and metadata storage with indexing capability.
- `UUID` native type for job/attempt/worker identities.
- `ON CONFLICT DO NOTHING / DO UPDATE` for idempotent writes.
- Advisory locks (optional future use for scheduler coordination).
- ACID transactions: atomic write of (job state + attempt state + job event) in one transaction.
- Different from LEDGER: demonstrates reasoning about why SQLite was appropriate there (single-writer batch pipeline) and why PostgreSQL is appropriate here (concurrent multi-writer workers). This is a portfolio-worthy distinction.

**Trade-offs:**
- Requires Docker or installed PostgreSQL (vs. embedded SQLite).
- Schema migrations required (using `golang-migrate/migrate`).
- Slightly more operational overhead than SQLite.
- `SKIP LOCKED` is specific to PostgreSQL (not portable to MySQL 5.x); acceptable.

**Rejected:**
- SQLite: single-writer architecture creates a bottleneck with concurrent workers. WAL mode allows concurrent reads but only one writer. Multiple workers attempting to claim jobs simultaneously would serialize or fail. This is the correct reason LEDGER uses SQLite (single pipeline writer) but not suitable here.
- MySQL: `SELECT FOR UPDATE SKIP LOCKED` added in MySQL 8.0 but is less battle-tested than PostgreSQL's implementation; ecosystem weaker for this pattern.
- MongoDB: no native `SKIP LOCKED` equivalent; achieving concurrent exclusive claim requires application-level coordination (compare-and-swap), which is error-prone.
- Redis: not durable by default; not suitable as sole authoritative store; appropriate as a secondary notification channel only.

**Operational consequences:**
- `docker compose up postgres` for local development.
- Migrations tracked via versioned `.sql` files in `/migrations`.
- Connection pooling via `pgxpool` (Go PostgreSQL driver).
- Health check: `SELECT 1` on `/healthz`.

---

### F-TDR-003 — Queue and broker: PostgreSQL SKIP LOCKED

**Requirement:** Job dispatch to workers; exclusive claim; recovery from missed dispatches; no silent authority split between queue and database; at-least-once delivery semantics.

**Candidates:**
1. Redis (pub/sub or Streams)
2. RabbitMQ (AMQP; durable queues)
3. Kafka (log-based; partitioned)
4. PostgreSQL SKIP LOCKED (database as queue)
5. In-memory channel (goroutine channel)

**Evaluation criteria:**
- Database-broker split problem (what happens if DB commits but publish fails)
- Claim exclusivity (exactly-one worker receives each job)
- Recovery behavior after broker failure
- Operational complexity
- Portfolio value (understanding broker trade-offs)
- At-least-once vs. exactly-once delivery

**Chosen: PostgreSQL SKIP LOCKED (database as queue)**

**Why:**

The central correctness concern with a separate message broker is the database-broker split:

```text
Scenario: API receives job submission
  Step 1: INSERT job into PostgreSQL → COMMIT ✓
  Step 2: PUBLISH message to broker → NETWORK FAILURE ✗

Result: Job is persisted but never dispatched.
Without recovery: job is stuck indefinitely.
With recovery: requires an outbox pattern or polling.
```

Using PostgreSQL as the queue eliminates this split entirely. The commit that persists the job IS the enqueue operation. There is no Step 2. No outbox pattern required.

`SKIP LOCKED` semantics:

```sql
SELECT * FROM jobs
WHERE state = 'QUEUED' AND eligible_at <= NOW()
ORDER BY priority DESC, queued_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED
```

- Two workers executing this simultaneously get different rows.
- No application-level mutex required.
- The lock is released automatically when the transaction commits.
- This is identical to what RabbitMQ's exclusive consumer semantics provide, but via a relational transaction.

Recovery is trivial:

```sql
SELECT * FROM jobs
WHERE state IN ('CLAIMED', 'RUNNING')
  AND lease_expires_at < NOW()
FOR UPDATE SKIP LOCKED
```

No broker-side dead-letter logic. No broker restart required. Jobs are visible in the same database.

**Trade-offs explicitly accepted:**
- Polling introduces latency between job enqueue and worker claim (configurable; default 1 second). Acceptable for FORGE's scale.
- Database handles both storage and dispatch load. At very high throughput (>10k jobs/second), a dedicated broker would outperform. This is out of FORGE's scope.
- Polling frequency is a tuning knob between latency (lower interval) and database load (higher interval).

**Rejected:**
- RabbitMQ: introduces DB-broker split problem. Solving it correctly requires a transactional outbox pattern (write to `outbox` table in same transaction, then a relay publishes to broker). This adds complexity without adding correctness guarantees that `SKIP LOCKED` doesn't already provide natively. RabbitMQ is the right tool when the broker is the primary channel and the database is secondary; in FORGE the database is authoritative.
- Kafka: log-based semantics (retain and replay) are mismatched with job queue semantics (claim and remove). Partition assignment creates additional complexity for job routing. Heavyweight for a portfolio project that must be reproducible locally.
- Redis: not durable by default. Redis Streams provide durable delivery but require a separate authoritative store for job state anyway. The split problem remains.
- In-memory channel: not durable; survives only within one process; cannot support multiple worker processes.

**Operational consequences:**
- No additional infrastructure beyond PostgreSQL.
- Worker poll interval configurable; default 1 second.
- Recovery sweep interval configurable; default 10 seconds.

---

### F-TDR-004 — Worker model: Separate processes

**Requirement:** Demonstrate worker crash and recovery; multiple concurrent workers; portfolio clarity about distributed execution.

**Candidates:**
1. Goroutines within the API process
2. Thread pool within the API process
3. Separate OS processes (worker binary)
4. Docker containers (one per worker)

**Evaluation criteria:**
- Crash demonstrability (can kill individual worker without affecting API)
- Process isolation (worker failure does not cascade to API)
- Realistic distributed model
- Portfolio demonstration clarity

**Chosen: Separate OS processes (worker binary)**

**Why:**

The primary demonstration scenario requires showing a worker crash. If workers are goroutines in the same process as the API, killing the worker kills the API. This is not representative of a real distributed system.

Separate worker processes:
- Can be killed with `kill -9 <pid>` or `docker kill forge-worker-1` to demonstrate crash without affecting the API or other workers.
- Have their own worker identity (`worker_id`), heartbeat loop, and lease tokens.
- Multiple workers can run concurrently on the same host (via Docker Compose `--scale`).
- The recovery scenario is fully demonstrable: kill one worker mid-execution, watch the lease expire, watch a second worker pick up the job.

**Trade-offs:**
- Requires coordination only through the shared database (no direct inter-process communication).
- More containers/processes to manage in Docker Compose.
- Worker startup cost per process (negligible for Go binaries).

**Rejected:**
- Goroutines within API process: cannot demonstrate crash without killing the API; not realistic.
- Thread pool: similar limitation; process-level crash kills all threads.
- Containers per worker: identical to separate processes from a code perspective; Docker Compose provides this naturally.

**Operational consequences:**
- Two binaries: `forge-api` and `forge-worker`.
- `forge-worker --name worker-1 --api-url http://forge-api:8080` for registration.
- Workers re-register on restart, reusing the same `worker_name`.

---

### F-TDR-005 — API transport: REST/HTTP + JSON

**Requirement:** Job submission, inspection, worker protocol; debuggable with curl; industry-standard interface.

**Candidates:**
1. REST/HTTP + JSON
2. gRPC (protobuf)
3. GraphQL

**Chosen: REST/HTTP + JSON**

**Why:**
- Demonstrable with `curl` in proof scripts.
- Idiomatic for the target use case (job submission, status inspection).
- Prometheus metrics endpoint over HTTP is standard.
- No code generation required (vs. protobuf).
- Easier to read in logs and debugging.

**Trade-offs:**
- Less efficient than gRPC for high-frequency worker heartbeats (HTTP/1.1 overhead).
- No streaming for long-polling.

**Rejected:** gRPC (adds code generation; less readable in logs; no benefit at FORGE's scale). GraphQL (overkill; querying job state doesn't benefit from graph traversal).

---

### F-TDR-006 — Deployment: Docker Compose

**Requirement:** Reproducible multi-process local environment; PostgreSQL; multiple workers; crash injection.

**Chosen: Docker Compose**

**Why:**
- Reproducible: `docker compose up` brings up the entire system.
- Individual container termination: `docker compose kill forge-worker-1` demonstrates crash.
- `scale` flag: `docker compose up --scale forge-worker=2` runs two concurrent workers.
- Different from LEDGER (Railway/Vercel deployment).
- No cloud account required.
- Standard industry tool for local distributed service development.

**Rejected:** Kubernetes (overkill; requires minikube or cloud cluster; no portfolio benefit over Docker Compose for this scope). Bare processes (less reproducible; requires manual PostgreSQL setup).

---

### F-TDR-007 — Observability: zerolog + Prometheus

**Requirement:** Structured logs with correlation identifiers; metrics for queue depth, execution latency, failure counts.

**Chosen:**
- `rs/zerolog` for structured JSON logging (zero-allocation, idiomatic Go)
- `prometheus/client_golang` for metrics
- Optional: Grafana + Prometheus in docker-compose for visual demonstration

**Why:**
- zerolog is lightweight and produces clean JSON with chainable field setting.
- `prometheus/client_golang` is the standard Go Prometheus client; integrates with the `/metrics` endpoint.
- Every log line carries `job_id`, `attempt_id`, `worker_id` via zerolog's context-based approach.

**Rejected:** Full OpenTelemetry stack (too heavy for this scope; adds complexity without proportionate portfolio value). Custom logging (reinventing the wheel; zerolog is idiomatic Go).

---

## G. Verification Architecture

### G.1 Overview

```text
                    FORGE Verification
                          │
       ┌──────────────────┼────────────────────┐
       ▼                  ▼                    ▼
    Static (V1)       Tests (V2-V4)       Evidence (V5)
       │                  │                    │
  type/lint/vet        unit/int/fail        product proof
                          │                    │
                          └──────────┬─────────┘
                                     ▼
                              machine-verifiable
                              scenario evidence
```

### G.2 V1 — Static verification

Tools: `go vet`, `golangci-lint`, `go build ./...`

Verifies:
- Type correctness across all packages
- Unused variables, unreachable code
- Linting (error handling, context propagation, lock alignment)
- Import graph (domain must not import infrastructure)
- All binaries compile cleanly

Evidence produced: exit code 0 from `make lint` and `make build`.

### G.3 V2 — Unit tests

Scope: pure domain logic; no database; no network.

Test targets:
- Job state machine: all legal transitions accepted; all illegal transitions rejected
- Attempt state machine: legal transitions only
- Lease validation: expired lease rejected; valid token accepted; invalid token rejected
- Retry policy: attempt count exhaustion produces FAILED; retries remaining produces QUEUED
- Backoff calculation: correct delay for attempt N
- Idempotency key logic: same key returns same job ID
- Failure classification: retryable vs. non-retryable failure categories
- Timeout calculation: eligible_at for retry attempts with backoff

Evidence produced: `go test ./internal/domain/... -v` with 100% pass and explicit scenario names.

### G.4 V3 — Integration tests

Scope: API handlers + real PostgreSQL (test database); no mocked infrastructure.

Test targets:
- Job submission → persisted in DB with correct initial state
- Duplicate submission with same idempotency_key → returns existing job
- Worker registration → persisted worker record
- Claim → job transitions QUEUED → CLAIMED; attempt created; lease set
- Claim concurrency: two goroutines claiming simultaneously → one succeeds, one blocks or gets different job
- Heartbeat renewal → lease_expires_at extended
- Success report → attempt SUCCEEDED; job SUCCEEDED; event created
- Failure report → attempt FAILED; if retries remain, job QUEUED with new attempt
- Recovery sweep → expired lease jobs requeued or failed
- Full job lifecycle: submit → claim → run → succeed (and verify event trail)
- Query endpoints: job by ID returns complete state; attempt history returns all attempts

Evidence produced: `go test ./tests/integration/... -v` with PostgreSQL in Docker.

### G.5 V4 — Failure verification

Scope: real system under deliberately injected failures.

Scenarios and verification:

| Scenario | Injection | Expected outcome | Verification |
|---|---|---|---|
| Worker crash | `kill -9` worker process mid-execution | Lease expires; attempt ABANDONED; new attempt created | Query job history: two attempts visible; second succeeds |
| Duplicate submission concurrent | Two goroutines submit with same idempotency_key simultaneously | Exactly one job created | Job count == 1; both submitters receive same job_id |
| Concurrent claim | Two workers polling simultaneously | Exactly one claim succeeds per job | No job has two CLAIMED/RUNNING attempts simultaneously |
| Stale lease completion | Worker completes after lease transferred | Completion rejected (lease_token mismatch) | API returns 409; job state unchanged |
| Retry exhaustion | Worker fails on every attempt | Job reaches FAILED after max_attempts | attempt_count == max_attempts; state == FAILED |
| Timeout | Worker holds lease without heartbeat | Lease expires; attempt TIMED_OUT or ABANDONED | Lease detection within configured window |
| Process restart (API) | Kill and restart API server | Jobs in flight survive; recovery continues | No jobs lost; state consistent after restart |
| DB write failure simulation | Inject DB error during completion | Transaction rolls back; job remains in previous state | Job remains RUNNING until next recovery cycle |

Evidence produced: `go test ./tests/failure/... -v` with evidence files per scenario.

### G.6 V5 — Product proof

Scope: full end-to-end deterministic demonstration against the real running system.

The proof is a script (`proof/run_proof.go` or `proof/run_proof.sh`) that:
1. Starts a clean system (Docker Compose)
2. Runs each demonstration scenario in sequence
3. Queries the system after each scenario
4. Asserts expected state
5. Outputs a machine-readable evidence file (`evidence/product-proof.json`)

The proof does not mock any component. It runs against real PostgreSQL, real API, real workers.

See Section H for the complete demonstration scenario definition.

Evidence produced: `evidence/product-proof.json` with scenario names, assertions, pass/fail, and job IDs.

---

## H. Product Demonstration

The final product proof is a deterministic script demonstrating all primary scenarios.

### H.0 Prerequisites

```text
docker compose up -d postgres forge-api
docker compose up -d --scale forge-worker=2
sleep 5  # allow registration
```

### Scenario A — Normal execution

```text
1. Submit job: POST /v1/jobs {type: "demo", payload: {...}}
   → receive job_id

2. Worker polls, claims job
   → job state: CLAIMED

3. Worker starts execution
   → job state: RUNNING

4. Worker completes successfully
   → job state: SUCCEEDED

5. Query history: GET /v1/jobs/{job_id}/history
   → 1 attempt; state: SUCCEEDED
   → events: SUBMITTED, CLAIMED, RUNNING, SUCCEEDED

Assert: terminal state == SUCCEEDED; attempt_count == 1
```

### Scenario B — Worker failure and recovery

```text
1. Submit job: POST /v1/jobs {type: "demo", ...}
   → receive job_id

2. Worker A claims and starts job
   → job state: RUNNING; attempt 1

3. Inject crash: docker kill forge-worker-1

4. Wait: lease_duration + recovery_interval (default: ~70 seconds)

5. Recovery scheduler detects expired lease
   → attempt 1 state: ABANDONED
   → job re-queued; attempt 2 created

6. Worker B claims and completes job
   → job state: SUCCEEDED

7. Query history: GET /v1/jobs/{job_id}/history
   → 2 attempts visible
   → attempt 1: ABANDONED; worker_id = worker-A
   → attempt 2: SUCCEEDED; worker_id = worker-B

Assert: terminal state == SUCCEEDED; attempt_count == 2; attempt 1 ABANDONED
```

### Scenario C — Retry exhaustion

```text
1. Submit job: POST /v1/jobs {type: "always-fail", max_attempts: 3, ...}

2. Worker claims; executes; reports failure (non-retryable or exhaustion)
   → attempt 1: FAILED; job QUEUED (retries remain)

3. Worker claims again; fails again
   → attempt 2: FAILED; job QUEUED (1 retry remaining)

4. Worker claims again; fails again
   → attempt 3: FAILED; job state: FAILED (retries exhausted)

5. No further claims occur for this job

Assert: terminal state == FAILED; attempt_count == 3 == max_attempts
```

### Scenario D — Duplicate submission

```text
1. Submit job: POST /v1/jobs {idempotency_key: "order-99", ...}
   → receive job_id_1

2. Submit same job: POST /v1/jobs {idempotency_key: "order-99", ...}
   → receive job_id_2

Assert: job_id_1 == job_id_2; exactly one job record exists
```

### Scenario E — Restart recovery

```text
1. Submit job; worker claims it (state: RUNNING)

2. Kill API server: docker kill forge-api

3. Restart API server: docker compose up forge-api

4. Verify: job still visible; state preserved

5. If lease expired during restart: recovery picks it up
   → job requeued; new attempt created
   → worker completes

Assert: no job lost after restart; state consistent with pre-crash observation
```

### Scenario F — Timeout

```text
1. Submit job with timeout_secs: 5, max_attempts: 1:
   POST /v1/jobs {type: "slow-job", timeout_secs: 5, max_attempts: 1, ...}

2. Worker claims; does NOT send heartbeat (simulated hang)

3. Lease expires after 5 + recovery_interval seconds

4. Recovery marks attempt TIMED_OUT; max_attempts exhausted
   → job state: TIMED_OUT (or FAILED)

Assert: terminal state in {TIMED_OUT, FAILED}; attempt_count == 1
```

### H.1 Evidence file format

```json
{
  "proof_version": "1.0",
  "run_at": "2026-09-24T...",
  "scenarios": [
    {
      "name": "normal_execution",
      "job_id": "...",
      "result": "PASS",
      "assertions": [
        {"check": "terminal_state == SUCCEEDED", "result": "PASS"},
        {"check": "attempt_count == 1", "result": "PASS"},
        {"check": "events contain SUBMITTED,CLAIMED,RUNNING,SUCCEEDED", "result": "PASS"}
      ]
    },
    ...
  ],
  "summary": {
    "total": 6,
    "passed": 6,
    "failed": 0
  }
}
```

---

## I. Implementation Plan

Implementation proceeds in controlled layers. Each layer has a defined objective, scope, contracts, and acceptance criteria. Later layers may not be started until the preceding layer's exit condition is met.

A layer must not introduce changes to prior layers' authoritative contracts without a documented reason. Implementation code is never authorized during the Architecture phase.

---

### Layer 1 — Foundation

**Objective:** Establish project structure, configuration, structured logging, database connectivity, and migration infrastructure.

**Dependencies:** None.

**Scope:**
- Go module initialization (`go mod init`)
- Directory structure per C.11 project structure
- Configuration loading (`FORGE_` prefix environment variables): database URL, listen address, lease duration, recovery interval, worker poll interval
- Structured logger (zerolog) with correlation-field middleware
- PostgreSQL connection pool (`pgxpool`)
- Migration runner (`golang-migrate/migrate`)
- Empty schema migration files (to be populated in Layer 3)
- Health check endpoint (`/healthz` → DB ping + scheduler liveness)
- Makefile targets: `make build`, `make test`, `make lint`, `make migrate-up`, `make migrate-down`
- Docker Compose skeleton: postgres + forge-api + forge-worker

**Allowed changes:** All foundational infrastructure code.
**Forbidden changes:** Domain logic, API handlers, worker logic, state machines.

**Contracts:**
- `Config` struct with all required fields and defaults
- Logger interface: `logger.With(ctx).Job(job_id).Attempt(attempt_id).Msg(...)`
- `Store` interface: empty initially; to be populated in Layer 3
- `/healthz` → `{"status": "ok", "database": "ok"}`

**Acceptance criteria:**
- `go build ./...` succeeds with no errors
- `go vet ./...` clean
- `make lint` clean
- `/healthz` returns 200 with real PostgreSQL connection
- `make migrate-up` runs without error

**Exit condition:** `/healthz` returns 200 in Docker Compose. `go build ./...` clean.

---

### Layer 2 — Domain Model

**Objective:** Define all domain entities, states, transitions, and invariants in pure Go code with no infrastructure dependencies.

**Dependencies:** Layer 1.

**Scope:**
- `internal/domain/job.go`: `Job` struct, `JobState` enum, all legal transitions as methods with invariant enforcement
- `internal/domain/attempt.go`: `ExecutionAttempt` struct, `AttemptState` enum, legal transitions
- `internal/domain/worker.go`: `Worker` struct, `WorkerState` enum
- `internal/domain/lease.go`: Lease validation logic (`IsExpired`, `IsOwner`, `MustRenew`)
- `internal/domain/policy.go`: `RetryPolicy` struct; `ShouldRetry(attempt_count, max_attempts) bool`; `NextEligibleAt(attempt_number, backoff_secs, multiplier) time.Time`
- `internal/domain/events.go`: `JobEvent` struct; `EventType` enum; all event types
- `internal/domain/errors.go`: domain error types (IllegalTransition, StaleLease, RetryExhausted)

**Allowed changes:** Domain package only.
**Forbidden changes:** Infrastructure, persistence, API handlers.

**Contracts:**
- `Job.Transition(to JobState) error` — returns `IllegalTransitionError` for invalid transitions
- `Lease.Validate(token uuid.UUID, now time.Time) error` — returns `StaleLeasError` for expired or mismatched
- `RetryPolicy.ShouldRetry(count, max int) bool`

**Acceptance criteria:**
- All legal job transitions exercised in unit tests
- All illegal transitions produce `IllegalTransitionError`
- Stale lease (expired) returns `StaleLeaseError`
- Stale lease (wrong token) returns `StaleLeaseError`
- `ShouldRetry` correct for all edge cases (0 attempts, max attempts, max+1)

**Evidence:** `go test ./internal/domain/... -v` all passing.

**Exit condition:** Domain unit tests 100% passing. No persistence imports in domain package.

---

### Layer 3 — Persistence

**Objective:** Implement PostgreSQL schema and repository layer. Define the authoritative storage contract.

**Dependencies:** Layers 1-2.

**Scope:**
- Schema migrations:
  - `001_create_jobs.sql`: jobs table with all fields, indexes, constraints
  - `002_create_execution_attempts.sql`
  - `003_create_workers.sql`
  - `004_create_job_events.sql`
  - `005_create_idempotency_keys.sql`
- `internal/store/store.go`: repository interface definitions:
  - `JobRepository`: CreateJob, GetJob, UpdateJobState, ListEligibleJobs, ClaimJob, etc.
  - `AttemptRepository`: CreateAttempt, UpdateAttemptState, ListAttempts, etc.
  - `WorkerRepository`: RegisterWorker, UpdateWorkerState, GetWorker, etc.
  - `EventRepository`: AppendEvent
  - `IdempotencyRepository`: Lookup, Record
- `internal/store/postgres/`: concrete implementations using `pgxpool`
- Transaction helper: `store.WithTx(ctx, func(tx Store) error)` for atomic multi-table writes
- `internal/store/postgres/claim.go`: `ClaimNextJob` using `SELECT FOR UPDATE SKIP LOCKED`
- `internal/store/postgres/recovery.go`: `FindExpiredLeases` using `SELECT FOR UPDATE SKIP LOCKED`

**Critical correctness requirements:**
- `ClaimNextJob` must be a single atomic transaction: SELECT FOR UPDATE SKIP LOCKED + UPDATE jobs + INSERT attempt + INSERT event
- State + event writes must be in the same transaction (not separate)
- `idempotency_keys` table uses ON CONFLICT behavior to serialize concurrent duplicate submissions

**Allowed changes:** Store interface and postgres implementation. Schema migrations.
**Forbidden changes:** Domain package, API handlers, worker logic.

**Contracts:**
- `Store.ClaimNextJob(ctx, workerID, leaseDuration) (*Job, *ExecutionAttempt, error)` — atomic
- `Store.CompleteAttempt(ctx, attemptID, leaseToken, result) error` — validates token before writing
- `Store.AbandonExpiredLeases(ctx, now) ([]RecoveredJob, error)` — atomic batch recovery

**Acceptance criteria:**
- All tables created correctly by migration
- `ClaimNextJob` called concurrently by 5 goroutines: each gets a different job (no duplicates)
- `CompleteAttempt` with wrong `leaseToken` returns error; job state unchanged
- Duplicate job submission with same `idempotency_key` returns existing job
- Transaction rollback on any write error leaves no partial state

**Evidence:** `go test ./internal/store/... -v` with real PostgreSQL.

**Exit condition:** Integration tests passing. Concurrent claim test passing.

---

### Layer 4 — Job Submission Service

**Objective:** Implement the submission API endpoint with idempotency, validation, and persistence.

**Dependencies:** Layers 1-3.

**Scope:**
- `internal/service/submission.go`: `SubmissionService.Submit(ctx, req) (*Job, error)`
  - Validate input (required fields, payload well-formed JSON)
  - Check idempotency key (if provided)
  - Create job record and event in one transaction
  - Return existing job if idempotency key already exists
- `internal/api/handlers/jobs.go`: `POST /v1/jobs` handler
- Input validation: job_type required; payload must be valid JSON; max_attempts >= 1; timeout_secs >= 1
- Response: `201 Created` with job record; `200 OK` if idempotency key matched existing job

**Allowed changes:** Service and API handler layers. No domain or store changes.

**Acceptance criteria:**
- `POST /v1/jobs {type: "test", payload: {}}` → 201 with job_id
- Same request with same `idempotency_key` → 200 with same job_id (not 201)
- Concurrent duplicate submissions → exactly one job created
- Invalid payload → 400 with error detail
- Persisted job is QUEUED with correct timestamps

**Exit condition:** Submission integration tests passing. Idempotency test passing.

---

### Layer 5 — Worker Registration and Heartbeat

**Objective:** Implement worker registration, heartbeat, and state management.

**Dependencies:** Layers 1-4.

**Scope:**
- `internal/service/worker_service.go`: Register, Heartbeat, Deregister
- `internal/api/handlers/workers.go`: `POST /v1/workers`, `POST /v1/workers/{id}/heartbeat`
- Worker state tracking (IDLE → BUSY → IDLE)
- Stale worker detection (last_heartbeat_at older than configured threshold)
- Worker re-registration with same name: update existing record
- `GET /v1/workers` and `GET /v1/workers/{id}` for inspection
- `internal/worker/client.go`: HTTP client for worker → API communication

**Acceptance criteria:**
- Worker registers → 201 with worker_id
- Worker re-registers with same name → 200 with same worker_id
- Heartbeat updates `last_heartbeat_at`
- Worker inactive for 2x heartbeat_interval → state transitions to STALE
- `GET /v1/workers` returns all registered workers with current state

**Exit condition:** Worker registration and heartbeat integration tests passing.

---

### Layer 6 — Dispatch (Worker Claim Protocol)

**Objective:** Implement the worker's job claim protocol using SKIP LOCKED.

**Dependencies:** Layers 1-5.

**Scope:**
- `internal/service/dispatch.go`: `DispatchService.ClaimNext(ctx, workerID) (*Job, *ExecutionAttempt, error)`
- Worker poll loop in `internal/worker/runner.go`:
  - Poll `ClaimNext` every `poll_interval`
  - On claim: start execution (Layer 7)
  - On no job available: sleep poll_interval
- `POST /v1/workers/{id}/claim` endpoint (or direct DB access from worker)
- Claim response includes `lease_token`, `lease_expires_at`, `attempt_id`

**Design note on claim architecture:** Workers can claim either via API call or by direct database access. Direct DB access requires sharing database credentials with workers. API-mediated claim is preferable for security (workers authenticate to API; only API has DB credentials). For v1.0, workers call the API to claim.

**Acceptance criteria:**
- Two workers polling simultaneously never claim the same job
- Claim updates job to CLAIMED with correct lease fields
- Claim creates attempt record atomically with job update
- `GET /v1/jobs/{id}` shows CLAIMED state and worker_id after claim
- Worker poll loop claims available jobs without manual intervention

**Exit condition:** Concurrent claim test passing (5 workers, 5 jobs → 1:1 mapping, no duplicates).

---

### Layer 7 — Execution Lifecycle

**Objective:** Implement execution start, heartbeat, success, and failure reporting.

**Dependencies:** Layers 1-6.

**Scope:**
- `internal/service/execution.go`:
  - `StartExecution(ctx, attemptID, leaseToken) error` → attempt CREATED → RUNNING
  - `RenewLease(ctx, jobID, leaseToken) error` → extend lease_expires_at
  - `CompleteExecution(ctx, attemptID, leaseToken, result) error` → attempt/job SUCCEEDED
  - `FailExecution(ctx, attemptID, leaseToken, reason, retryable bool) error` → fail attempt; retry or terminal
- `internal/api/handlers/attempts.go`:
  - `POST /v1/attempts/{id}/start`
  - `POST /v1/attempts/{id}/heartbeat`
  - `POST /v1/attempts/{id}/complete`
  - `POST /v1/attempts/{id}/fail`
- Lease token validation on every operation (stale lease → 409 Conflict)
- Atomic transaction: attempt state + job state + event in one write
- Simulated work in `internal/worker/executor.go`: configurable sleep with heartbeat loop

**Acceptance criteria:**
- `CompleteExecution` with wrong lease_token → 409; job state unchanged
- `CompleteExecution` with correct token → job SUCCEEDED; event created
- `FailExecution` with retries remaining → job QUEUED; new attempt created
- `FailExecution` at max_attempts → job FAILED
- `GET /v1/jobs/{id}/history` after success shows complete event trail

**Exit condition:** Full job lifecycle integration test (submit → claim → start → heartbeat → complete) passing.

---

### Layer 8 — Recovery Scheduler

**Objective:** Implement the background scheduler that detects expired leases and initiates recovery.

**Dependencies:** Layers 1-7.

**Scope:**
- `internal/scheduler/scheduler.go`: background goroutine running on `recovery_interval`
- `RecoverStaleClaims(ctx) (int, error)`:
  - SELECT expired leases FOR UPDATE SKIP LOCKED
  - For each: abandon attempt, increment attempt_count, requeue or terminate
  - All writes in one transaction per batch
- `EnforceTimeouts(ctx) (int, error)`:
  - SELECT jobs running longer than `timeout_secs`
  - Same recovery logic but terminal state is TIMED_OUT
- Scheduler started by API server on startup; stopped on shutdown
- Scheduler heartbeat: periodic log line with recovery count
- `/healthz` reports scheduler last run time

**Acceptance criteria:**
- Worker claims job; `kill -9` worker; wait lease_duration + recovery_interval
- Job transitions: attempt ABANDONED; job QUEUED (retries remain)
- Second worker picks up job and completes
- Query history: two attempts visible; first ABANDONED, second SUCCEEDED
- Retry exhaustion via scheduler: attempt_count reaches max_attempts; job FAILED

**Exit condition:** Worker crash + recovery test passing in Docker Compose.

---

### Layer 9 — Retry Policy and Backoff

**Objective:** Implement configurable retry policy with backoff support.

**Dependencies:** Layers 1-8.

**Scope:**
- Retry policy enforcement in `FailExecution` and `RecoverStaleClaims`
- `eligible_at = NOW() + backoff_secs * multiplier^(attempt_number-1)`
- Job-level override via submission request
- Default policy: max_attempts=3, backoff_secs=0, multiplier=1.0
- Explicit non-retryable failure classification (worker can mark failure as non-retryable → immediate terminal)

**Acceptance criteria:**
- Job with backoff_secs=10, attempt 1 fails: `eligible_at` is ~10 seconds in future
- Job not claimed until `eligible_at <= NOW()`
- Non-retryable failure on attempt 1 with max_attempts=3 → job immediately FAILED

**Exit condition:** Retry policy unit tests passing. Integration test with backoff passing.

---

### Layer 10 — API and Query Layer

**Objective:** Complete the REST API with all inspection endpoints.

**Dependencies:** Layers 1-9.

**Scope:**
- `GET /v1/jobs/{id}` — full job record with current state
- `GET /v1/jobs/{id}/history` — all attempts + events in chronological order
- `GET /v1/jobs/{id}/attempts` — list of all attempt records
- `GET /v1/jobs/{id}/attempts/{attempt_id}` — single attempt detail
- `GET /v1/workers` — all workers with state
- `GET /v1/workers/{id}` — single worker
- `GET /v1/queue/stats` — queue depth, active jobs, succeeded/failed counts
- Middleware: request_id injection, structured access logging
- Response envelope: consistent JSON structure with `data`, `error`, `meta`
- Pagination for list endpoints

**Acceptance criteria:**
- All endpoints return correct data after each scenario
- `GET /v1/jobs/{id}/history` after worker crash shows both attempts
- `GET /v1/queue/stats` reflects real-time queue state

**Exit condition:** All API endpoints tested with integration tests.

---

### Layer 11 — Observability

**Objective:** Complete structured logging and Prometheus metrics.

**Dependencies:** Layers 1-10.

**Scope:**
- Every state transition emits zerolog JSON with `job_id`, `attempt_id`, `worker_id`, `from_state`, `to_state`
- Every API request emits access log with `request_id`, `method`, `path`, `status`, `duration_ms`
- Prometheus metrics:
  - `forge_jobs_submitted_total` (counter)
  - `forge_jobs_queued` (gauge)
  - `forge_jobs_running` (gauge)
  - `forge_jobs_succeeded_total` (counter)
  - `forge_jobs_failed_total` (counter)
  - `forge_attempts_abandoned_total` (counter, label: reason=lease_expired|timeout)
  - `forge_recovery_runs_total` (counter)
  - `forge_jobs_recovered_total` (counter)
  - `forge_job_execution_duration_seconds` (histogram)
  - `forge_job_queue_wait_duration_seconds` (histogram)
  - `forge_lease_renewals_total` (counter)
  - `forge_lease_stale_rejections_total` (counter)
- `/metrics` endpoint exposing Prometheus text format
- Optional: Grafana + Prometheus in docker-compose

**Acceptance criteria:**
- After running Scenario B, `forge_attempts_abandoned_total` > 0
- After running Scenario C, `forge_jobs_failed_total` > 0
- `forge_jobs_queued` gauge reflects actual queue depth
- All log lines for state transitions include required fields

**Exit condition:** All metrics populated correctly after running scenarios A-F.

---

### Layer 12 — Failure Verification

**Objective:** Deliberately inject each failure mode and verify correct behavior.

**Dependencies:** Layers 1-11.

**Scope:**
- Test suite in `tests/failure/`
- Failure injection helpers:
  - `KillWorker(containerName)` → `docker kill`
  - `PauseWorker(containerName)` → `docker pause` (simulates slow/hung worker)
  - `SimulateSlowHeartbeat(workerID)` → bypass heartbeat
  - `InjectDBError(table, op)` → PostgreSQL fault injection (optional)
- Each scenario in H (A-F) exercised as a test with assertions
- Concurrent claim test: 10 workers, 10 jobs, verify 1:1 claim mapping
- Restart test: kill API + postgres, restart, verify no state loss
- Stale lease rejection: manually expire lease, verify completion rejected

**Acceptance criteria:**
- All 6 demonstration scenarios produce correct terminal state
- No job is simultaneously claimed by two workers under any concurrent load
- After worker crash, job is recoverable and eventually SUCCEEDED or FAILED (not stuck)
- After API restart, all jobs visible with correct state

**Exit condition:** `go test ./tests/failure/... -v` all passing in Docker Compose environment.

---

### Layer 13 — Product Proof

**Objective:** Produce the reproducible, machine-verifiable product proof.

**Dependencies:** Layers 1-12.

**Scope:**
- `proof/run_proof.go`: self-contained proof script
- Runs all 6 scenarios sequentially
- Asserts expected state after each scenario
- Outputs `evidence/product-proof.json`
- `make proof` target that:
  1. `docker compose down -v && docker compose up -d`
  2. Runs migrations
  3. Starts proof script
  4. Prints summary and evidence path
- `evidence/` directory committed with sample proof output

**Acceptance criteria:**
- `make proof` completes with 0 failures
- `evidence/product-proof.json` contains all 6 scenarios with PASS
- Proof is reproducible by another engineer with only `make proof`

**Exit condition:** `make proof` succeeds from clean Docker state. Evidence file committed.

---

## J. Risk Register

### Blocking risks

**RISK-001 — Concurrent claim correctness**
Category: Blocking
Description: `SELECT FOR UPDATE SKIP LOCKED` behavior under concurrent load must be verified empirically. If PostgreSQL allows two workers to claim the same job under any configuration, the ownership invariant (F-INV-004) is violated.
Mitigation: V4 test with 10 concurrent workers claiming 10 jobs verifies this before any other scenario. Must pass before Layer 6 exit.
Detection: V4 concurrent claim test.

**RISK-002 — Lease token rotation gap**
Category: Blocking
Description: If the recovery scheduler and a stale worker attempt to complete the same job in an unguarded window, the wrong outcome may be committed.
Mitigation: The `lease_token` field is rotated atomically with recovery. The `CompleteExecution` function validates the token in the same transaction that writes the state. No race window exists if implemented correctly.
Detection: V4 stale lease rejection test.

---

### High risks

**RISK-003 — At-least-once re-execution**
Category: High
Description: FORGE makes no guarantee of exactly-once execution. A worker that completes work and crashes before reporting will cause the work to be re-executed on retry. This is not a bug — it is an explicit architectural decision — but it must be documented clearly in the product proof and demonstration narrative.
Mitigation: Explicitly document in Architecture.md (this document), proof script commentary, and worker executor.
Detection: Scenario B explicitly demonstrates this.

**RISK-004 — Recovery sweep timing under load**
Category: High
Description: If many jobs have expired leases simultaneously, a single recovery sweep may time out or be slow. Jobs may remain in stale state longer than expected.
Mitigation: Recovery sweep uses `LIMIT 100` per run; runs every configurable interval; processes in batch transactions.
Detection: V4 bulk failure test.

**RISK-005 — Idempotency key scope**
Category: High
Description: Idempotency keys are global (not scoped per job_type). A key collision across different job types would return the wrong job.
Mitigation: Scope idempotency key uniqueness per `(job_type, idempotency_key)` not globally. Defined explicitly in schema.
Detection: Unit test for cross-type key isolation.

---

### Medium risks

**RISK-006 — Worker re-registration identity**
Category: Medium
Description: When a worker restarts and re-registers with the same `worker_name`, it must reuse the same `worker_id` to maintain attempt history linkage. If a new `worker_id` is generated on restart, the old attempts are orphaned.
Mitigation: `RegisterWorker(name)` performs an `INSERT ON CONFLICT(worker_name) DO UPDATE` returning the existing `worker_id`.
Detection: V4 restart test verifying worker_id continuity.

**RISK-007 — Clock skew between workers**
Category: Medium
Description: If worker clocks are skewed relative to the database, lease expiry calculations may be incorrect (a worker's clock says it has time; the database clock says the lease has expired).
Mitigation: All time comparisons use the database clock (`NOW()` in SQL queries). Worker provides only durations (heartbeat interval), not absolute timestamps. `lease_expires_at` is set by the database.
Detection: Architecture enforced; no separate test needed.

**RISK-008 — Payload size limits**
Category: Medium
Description: JSONB payloads stored in PostgreSQL. Large payloads increase memory pressure on workers and API.
Mitigation: Enforce a maximum payload size (e.g., 1MB) at API ingestion. Document limit explicitly.
Detection: V3 boundary test.

---

### Low risks

**RISK-009 — Go module dependency management**
Category: Low
Description: Third-party dependencies (pgxpool, zerolog, prometheus/client_golang, golang-migrate) may have breaking changes.
Mitigation: Pin exact versions in `go.sum`. Audit dependencies before Layer 1 exit.

**RISK-010 — Docker Compose networking**
Category: Low
Description: Worker containers must resolve `forge-api` hostname. Incorrect Docker network configuration would prevent registration.
Mitigation: All services on the same Docker Compose network. Verified in Layer 1 Docker Compose setup.

**RISK-011 — Proof script ordering sensitivity**
Category: Low
Description: The product proof runs scenarios sequentially. If a prior scenario leaves unexpected state, later scenarios may fail.
Mitigation: Each scenario submits fresh jobs with unique identifiers. System-level cleanup between scenarios if needed.

---

## K. Engineering Invariants

**F-INV-001 — Valid Lifecycle**
No job state transition is legal unless it appears in the state machine transition table in Section E.1. Every transition is enforced by the domain layer before the persistence layer is called.

**F-INV-002 — Durable Identity**
Every accepted job submission receives a UUID that is persisted before the API response is sent. No job is accepted without persistence.

**F-INV-003 — Submission Idempotency**
A submission with the same `idempotency_key` never creates more than one job record. The `idempotency_keys` table unique constraint enforces this at the database level; the service layer enforces it at the application level.

**F-INV-004 — Ownership Safety**
At most one valid active execution owner exists for a job at any time. Enforced by: (a) `SELECT FOR UPDATE SKIP LOCKED` on claim; (b) `lease_token` rotation on recovery; (c) token validation on every attempt completion, failure, and heartbeat.

**F-INV-005 — Bounded Retry**
Attempt count never exceeds `max_attempts`. Enforced by the retry policy check before creating a new attempt.

**F-INV-006 — Historical Preservation**
Execution attempts are never deleted or overwritten. Attempt 1 remains visible after attempt 2 is created. The `execution_attempts` table has no DELETE operations in normal operation.

**F-INV-007 — Recoverability**
Any job in state CLAIMED or RUNNING with `lease_expires_at < NOW()` is detectable by the recovery scheduler and will eventually be recovered (requeued or terminated). The recovery scheduler runs on a bounded interval.

**F-INV-008 — Terminal Integrity**
A job in state SUCCEEDED, FAILED, or TIMED_OUT cannot transition to any other state. Enforced by the domain transition guard in `Job.Transition()`.

**F-INV-009 — Authoritative State**
PostgreSQL is the sole authoritative source of truth for all job, attempt, worker, and event state. No component holds authoritative state in memory. Worker in-memory state is derived from the database; if it diverges, the database wins.

**F-INV-010 — Explainability**
Every job in a terminal state has: at least one execution attempt record, at least one job event for each state transition, worker identity for each attempt, timestamps for each transition, and failure reason for each failed attempt. `GET /v1/jobs/{id}/history` answers "what happened to this job?" with evidence.

**Invariant enforcement matrix:**

| Invariant | Primary enforcement | Secondary enforcement | V-level |
|---|---|---|---|
| F-INV-001 | `domain.Job.Transition()` | API handler rejects invalid requests | V2, V4 |
| F-INV-002 | `store.CreateJob()` (INSERT, not upsert) | API 500 on DB failure (no silent drop) | V3 |
| F-INV-003 | `idempotency_keys` UNIQUE constraint | Service-layer lookup before INSERT | V3, V4 |
| F-INV-004 | `SELECT FOR UPDATE SKIP LOCKED` + `lease_token` validation | Recovery token rotation | V3, V4 |
| F-INV-005 | `RetryPolicy.ShouldRetry()` before attempt creation | `max_attempts` constraint in DB | V2, V4 |
| F-INV-006 | No DELETE on `execution_attempts`; append-only INSERT | Code review | V3 |
| F-INV-007 | Recovery scheduler sweep query | Test: kill worker; wait; verify recovery | V4 |
| F-INV-008 | `Job.Transition()` returns error for terminal → non-terminal | DB CHECK constraint on state (optional) | V2, V4 |
| F-INV-009 | Workers have no persistent state; all reads from DB | Architecture enforced | V4 (restart test) |
| F-INV-010 | `AppendEvent` called in same tx as state writes | Query returns non-empty history | V3, V5 |

---

## L. Project Structure

```text
forge/
├── cmd/
│   ├── forge-api/
│   │   └── main.go           ← API server + embedded scheduler entry point
│   └── forge-worker/
│       └── main.go           ← Worker process entry point
├── internal/
│   ├── domain/
│   │   ├── job.go            ← Job entity, JobState, transitions
│   │   ├── attempt.go        ← ExecutionAttempt, AttemptState
│   │   ├── worker.go         ← Worker entity, WorkerState
│   │   ├── lease.go          ← Lease validation, expiry
│   │   ├── policy.go         ← RetryPolicy, backoff
│   │   ├── events.go         ← JobEvent, EventType
│   │   └── errors.go         ← IllegalTransition, StaleLease, RetryExhausted
│   ├── service/
│   │   ├── submission.go     ← SubmissionService
│   │   ├── dispatch.go       ← DispatchService
│   │   ├── execution.go      ← ExecutionService
│   │   ├── recovery.go       ← RecoveryService
│   │   ├── worker_service.go ← WorkerService
│   │   └── query.go          ← QueryService (read-only)
│   ├── store/
│   │   ├── store.go          ← Repository interfaces
│   │   └── postgres/
│   │       ├── jobs.go
│   │       ├── attempts.go
│   │       ├── workers.go
│   │       ├── events.go
│   │       ├── idempotency.go
│   │       ├── claim.go      ← SKIP LOCKED claim implementation
│   │       └── recovery.go   ← Expired lease query
│   ├── api/
│   │   ├── server.go         ← HTTP server setup, middleware
│   │   └── handlers/
│   │       ├── jobs.go
│   │       ├── attempts.go
│   │       ├── workers.go
│   │       └── health.go
│   ├── worker/
│   │   ├── runner.go         ← Poll loop, claim, execute
│   │   ├── executor.go       ← Job type dispatch, simulated work
│   │   └── client.go         ← HTTP client for worker→API
│   ├── scheduler/
│   │   └── scheduler.go      ← Recovery sweep, timeout enforcement
│   └── telemetry/
│       ├── logger.go         ← zerolog setup, context propagation
│       └── metrics.go        ← Prometheus metric definitions
├── migrations/
│   ├── 001_create_jobs.sql
│   ├── 002_create_execution_attempts.sql
│   ├── 003_create_workers.sql
│   ├── 004_create_job_events.sql
│   └── 005_create_idempotency_keys.sql
├── tests/
│   ├── integration/          ← Real PostgreSQL; no mocks
│   ├── failure/              ← Docker Compose; crash injection
│   └── proof/                ← Product proof assertions
├── proof/
│   └── run_proof.go          ← Deterministic demonstration script
├── evidence/                 ← Committed proof outputs
│   └── .gitkeep
├── deploy/
│   ├── docker-compose.yml
│   └── docker-compose.override.yml  ← Local dev overrides
├── Makefile
├── go.mod
├── go.sum
├── Architecture.md           ← This document
└── README.md
```

---

## M. Executive Summary

**What FORGE is:** A distributed job execution platform demonstrating deliberate semantics for concurrent ownership, asynchronous dispatch, failure detection, retry policy, and execution evidence.

**What makes it different from LEDGER:** LEDGER is a deterministic batch pipeline with a single sequential processor. FORGE is a multi-process distributed system where workers, API servers, schedulers, and the database interact concurrently and can fail independently.

**The central problem:** How do you reliably execute asynchronous work when any component can fail at any point?

**The architectural answer:** Make PostgreSQL the single authoritative source of truth for both job state and queue state. Use `SELECT FOR UPDATE SKIP LOCKED` for atomic exclusive claim. Use a rotating `lease_token` to invalidate stale workers. Use a recovery scheduler to detect lease expiry and initiate recovery. Document at-least-once semantics explicitly. Preserve every attempt and every event. Never let ephemeral state become authoritative.

**Key technology decisions:**
- **Go** — not Python; goroutines; compiled binaries; process isolation for crash demonstration
- **PostgreSQL** — not SQLite; concurrent writes; SKIP LOCKED; row-level locking
- **No separate message broker** — PostgreSQL IS the queue; eliminates DB-broker split problem; documented trade-off
- **Separate worker processes** — individually killable; demonstrates real distributed failure
- **Docker Compose** — reproducible; multi-process; crash injection via `docker kill`

**Questions this system can answer:**

| Question | Answer |
|---|---|
| What happens if the worker dies after claiming? | Lease expires; recovery requeues after detection window |
| What prevents two workers executing the same job? | `SELECT FOR UPDATE SKIP LOCKED` makes claim atomic and exclusive |
| What happens if the queue delivers twice? | There is no separate queue; PostgreSQL claim is idempotent via transaction |
| What happens if DB succeeds but publish fails? | There is no separate publish step; DB commit IS the enqueue |
| What happens if the worker succeeds but dies before reporting? | Lease expires; retry created; work may re-execute (at-least-once, explicitly stated) |
| What exactly does retry mean? | New ExecutionAttempt record; previous attempt preserved; job returned to QUEUED |
| What is authoritative? | PostgreSQL only. Workers have no authoritative state |
| Can the system recover after restart? | Yes. All state is in PostgreSQL; no in-memory authority |
| What guarantees does the system provide? | At-least-once execution; exactly-once state transitions (per transaction); bounded retry |
| Why PostgreSQL not RabbitMQ? | Eliminates DB-broker split; SKIP LOCKED provides same exclusive claim semantics |
| Why PostgreSQL not SQLite? | Multiple concurrent writers require row-level locking; SQLite is single-writer |
| How was recovery proved? | V4 failure test: kill worker; wait lease_duration; verify attempt ABANDONED; verify requeue; verify second worker completes |
| What happens when retries exhausted? | attempt_count == max_attempts; job transitions to FAILED; no further claims |
| Can you reproduce failure deterministically? | Yes. `make proof` runs all 6 scenarios against real Docker Compose system; outputs machine-verifiable evidence |

**The system is ready for implementation when this specification can be defended without revision.**

---

## N. Open Decisions

Before implementation begins, the following require explicit resolution:

**OD-001 — Worker claim protocol: API-mediated vs. direct DB**
Workers can claim jobs either by calling the API (`POST /v1/workers/{id}/claim`) or by connecting directly to PostgreSQL with `SKIP LOCKED`. API-mediated claim is preferred for security (workers don't need DB credentials) but adds latency. Direct DB claim is more efficient but requires credential distribution.
*Recommendation: API-mediated for v1.0. Simpler security model. Direct DB access is a v2 optimization.*

**OD-002 — Heartbeat model: worker-level vs. attempt-level**
Heartbeats can be sent at the job level (renewing the job's lease) or the attempt level (renewing the attempt's lease). Attempt-level is more granular and maps cleanly to the lease model.
*Recommendation: Attempt-level heartbeat. Worker sends `POST /v1/attempts/{id}/heartbeat` with `lease_token`.*

**OD-003 — Scheduler placement: embedded vs. separate process**
The recovery scheduler can run as a goroutine inside the API server or as a separate binary.
*Recommendation: Embedded goroutine in v1.0. Simpler deployment. Separate binary if scheduler needs independent scaling (out of scope).*

**OD-004 — Job cancellation**
The brief mentions cancellation as a possible transition. Including it adds complexity (race between in-progress worker and cancellation request).
*Recommendation: Exclude from v1.0 scope. Define state machine slot (CANCELLED terminal state) but do not implement handler.*

**OD-005 — Worker capabilities filtering**
Workers could declare job_types they handle; dispatch would filter eligible jobs by type.
*Recommendation: Implement in dispatch query as `WHERE job_type = ANY($worker_capabilities)`. Simple; no extra infrastructure.*

---

## Status

**Architecture:** v1.0 baseline
**Implementation status:** Not started
**Current frontier:** Architecture complete; ready for implementation review
**Not implemented:** All layers (by instruction)

The implementation plan (Section I) defines 13 layers from Foundation to Product Proof. No implementation code may be written until this specification is reviewed and approved.
