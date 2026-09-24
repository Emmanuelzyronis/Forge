-- FORGE schema v1 — Layer 3
-- TIMESTAMPTZ everywhere; UTC enforced by the application layer.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- workers: registered execution agents
CREATE TABLE IF NOT EXISTS workers (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT        NOT NULL,
    hostname          TEXT        NOT NULL DEFAULT '',
    pid               INTEGER     NOT NULL DEFAULT 0,
    state             TEXT        NOT NULL DEFAULT 'REGISTERED',
    capabilities      TEXT[]      NOT NULL DEFAULT '{}',
    registered_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_job_id       UUID,
    CONSTRAINT workers_name_key UNIQUE (name)
);

-- jobs: the authoritative job queue and state store.
-- PostgreSQL is both the queue and the state store; no separate broker (F-TDR-003).
CREATE TABLE IF NOT EXISTS jobs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key     TEXT        UNIQUE,
    kind                TEXT        NOT NULL,
    payload             JSONB       NOT NULL DEFAULT '{}',
    state               TEXT        NOT NULL DEFAULT 'QUEUED',
    priority            INTEGER     NOT NULL DEFAULT 0,
    max_attempts        INTEGER     NOT NULL DEFAULT 3,
    attempt_count       INTEGER     NOT NULL DEFAULT 0,
    timeout_secs        INTEGER     NOT NULL DEFAULT 300,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    queued_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    eligible_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    terminal_at         TIMESTAMPTZ,
    current_attempt_id  UUID,
    lease_token         UUID,
    lease_expires_at    TIMESTAMPTZ,
    last_heartbeat_at   TIMESTAMPTZ,
    correlation_id      TEXT
);

-- Partial index for the dispatch query: QUEUED jobs ordered by priority then age.
-- SKIP LOCKED reads this index to find candidates without scanning all jobs.
CREATE INDEX IF NOT EXISTS idx_jobs_dispatch
    ON jobs (priority DESC, queued_at ASC)
    WHERE state = 'QUEUED';

-- Partial index for the recovery scheduler: expired leases on in-flight jobs.
CREATE INDEX IF NOT EXISTS idx_jobs_recovery
    ON jobs (lease_expires_at)
    WHERE state IN ('CLAIMED', 'RUNNING');

-- job_attempts: one row per execution attempt.
-- attempt_num is 1-indexed and monotonically increasing per job.
CREATE TABLE IF NOT EXISTS job_attempts (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id         UUID        NOT NULL REFERENCES jobs(id),
    attempt_num    INTEGER     NOT NULL,
    worker_id      UUID        NOT NULL REFERENCES workers(id),
    state          TEXT        NOT NULL DEFAULT 'CREATED',
    lease_token    UUID,
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ,
    error_detail   TEXT,
    failure_detail JSONB,
    result         JSONB,
    duration_ms    BIGINT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT job_attempts_job_num_key UNIQUE (job_id, attempt_num)
);

CREATE INDEX IF NOT EXISTS idx_attempts_job ON job_attempts (job_id);

-- job_events: immutable audit trail; never updated or deleted (F-INV-010).
-- Written in the same transaction as the state change it records.
CREATE TABLE IF NOT EXISTS job_events (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id         UUID        NOT NULL REFERENCES jobs(id),
    attempt_id     UUID        REFERENCES job_attempts(id),
    worker_id      UUID        REFERENCES workers(id),
    type           TEXT        NOT NULL,
    from_state     TEXT,
    to_state       TEXT,
    metadata       JSONB,
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    correlation_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_events_job ON job_events (job_id, occurred_at);
