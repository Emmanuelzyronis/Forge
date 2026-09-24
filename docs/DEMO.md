# FORGE Demo — Crash Recovery

This document describes how to run the reproducible crash-recovery scenario, collect evidence, and render the portfolio video.

## What the demo proves

Worker-01 is killed with `SIGKILL` while executing a job. No sentinel write is required. The lease expires by inaction. The embedded recovery scheduler detects the expired lease, marks the attempt ABANDONED, and re-queues the job. Worker-02 picks it up and completes it successfully.

The full event history — SUBMITTED, CLAIMED (×2), STARTED (×2), ABANDONED, REQUEUED, SUCCEEDED — is written atomically to `job_events` alongside every state change, providing a complete, auditable trail.

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Docker + Docker Compose | v2+ | For postgres + forge-api |
| Go | 1.25 | Must be at `/usr/local/go/bin/go` |
| jq | any | Used in the script |
| Node.js | 18+ | For Remotion video rendering |

## Running the demo

```bash
make demo-crash
```

This runs `demo/scripts/demo-crash.sh` which:

1. **Resets** the environment — `docker compose down -v`
2. **Starts** postgres + forge-api in Docker (no workers)
3. **Builds** the worker binary to `/tmp/forge-worker`
4. **Starts** worker-01 as a local process (`FORGE_WORKER_NAME=worker-01`)
5. **Submits** a `demo.crash-recovery` job (`max_attempts=3`, `timeout_secs=60`)
6. **Waits** for the job to reach RUNNING
7. **Kills** worker-01 with `SIGKILL`
8. **Waits** for the recovery scheduler to re-queue (~40s: 30s lease + 10s sweep)
9. **Starts** worker-02 (`FORGE_WORKER_NAME=worker-02`)
10. **Waits** for the job to SUCCEED
11. **Collects** evidence from the API → `demo/evidence/`

## Evidence output

```
demo/evidence/
├── raw-evidence.json       # raw API responses: job, events, attempts, workers
└── presentation.json       # normalized for Remotion: timeline + stats
```

### presentation.json structure

```json
{
  "scenario": "worker-crash-recovery",
  "timeline": [
    { "elapsed_secs": 0.0,  "event_type": "SUBMITTED", "to_state": "QUEUED" },
    { "elapsed_secs": 1.1,  "event_type": "CLAIMED",   "worker_name": "worker-01" },
    { "elapsed_secs": 1.3,  "event_type": "STARTED",   "worker_name": "worker-01" },
    { "elapsed_secs": 4.9,  "event_type": "CRASH",     "is_crash": true, "worker_name": "worker-01" },
    { "elapsed_secs": 42.7, "event_type": "ABANDONED",  "is_recovery": true },
    { "elapsed_secs": 43.1, "event_type": "CLAIMED",    "worker_name": "worker-02" },
    { "elapsed_secs": 62.1, "event_type": "SUCCEEDED",  "worker_name": "worker-02" }
  ],
  "stats": {
    "total_duration_secs": 62.1,
    "recovery_latency_secs": 38,
    "lease_duration_secs": 30,
    "recovery_interval_secs": 10
  }
}
```

## Collect evidence only (without re-running demo)

```bash
bash demo/scripts/collect-evidence.sh <job-id>
```

## Rendering the portfolio video

```bash
make demo-video
```

Installs Remotion dependencies (first run), renders `ForgeDemo` composition to `remotion/out/forge-demo.mp4`.

To use the fixture (no live run required):

```bash
cd remotion && FORGE_FIXTURE=../demo/fixtures/worker-crash-recovery.json npm run render:fixture
```

### Remotion development mode

```bash
cd remotion && npm install && npm start
# opens Remotion Studio at http://localhost:3000
```

The studio loads `demo/evidence/presentation.json` if it exists, otherwise falls back to `demo/fixtures/worker-crash-recovery.json`.

## Demo parameters

| Config | Value | Notes |
|--------|-------|-------|
| Lease duration | 30s | `FORGE_LEASE_DURATION_SECS` |
| Recovery sweep | 10s | `FORGE_RECOVERY_INTERVAL_SECS` |
| Heartbeat | 8s | `FORGE_HEARTBEAT_INTERVAL_SECS` |
| Job kind | `demo.crash-recovery` | runs for 12s |
| max_attempts | 3 | crash uses attempt 1, recovery uses attempt 2 |
| Expected recovery latency | ~38-42s | lease(30s) + sweep(10s) + jitter |

## Secondary scenario — stale worker rejection

A `demo.stale-worker` job kind also runs for 12s and can be used to demonstrate F-INV-006: after recovery abandons the attempt, the original worker's Succeed call is rejected with 409 Conflict because the lease token has been rotated.

## Invariants demonstrated

| Invariant | Description |
|-----------|-------------|
| F-INV-001 | Exactly-one claim: concurrent workers can't both claim the same job |
| F-INV-006 | Stale worker rejection: old lease token → 409 Conflict |
| F-INV-007 | Double-recovery prevention: SELECT FOR UPDATE in recovery transaction |
| F-INV-009 | Heartbeat prevents recovery for live workers |
| F-INV-010 | Atomic state + event: neither commits without the other |
