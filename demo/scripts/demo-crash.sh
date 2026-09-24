#!/usr/bin/env bash
# FORGE Crash-Recovery Demo
#
# Demonstrates: worker crash mid-job → lease expiry → recovery scheduler
# re-queues → second worker picks up → SUCCEEDED
#
# Prerequisites:
#   - Docker + Docker Compose
#   - Go 1.25 at /usr/local/go/bin/go
#   - jq  (brew install jq / apt install jq)
#   - FORGE repo built once: make build
#
# Usage:
#   bash demo/scripts/demo-crash.sh [--no-rebuild]
#
# Output:
#   demo/evidence/raw-evidence.json      — raw API responses
#   demo/evidence/presentation.json      — normalized for Remotion
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="PATH=$PATH:/usr/local/go/bin go"
BASE="http://localhost:8081"
EVIDENCE_DIR="$REPO_ROOT/demo/evidence"
DOCKER="docker compose -f $REPO_ROOT/deploy/docker-compose.yml"
REBUILD=true

for arg in "$@"; do
  case $arg in --no-rebuild) REBUILD=false ;; esac
done

# ─── Colours ────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

step()  { echo -e "\n${CYAN}${BOLD}▶ $*${RESET}"; }
ok()    { echo -e "  ${GREEN}✓ $*${RESET}"; }
warn()  { echo -e "  ${YELLOW}⚠ $*${RESET}"; }
fail()  { echo -e "  ${RED}✗ $*${RESET}"; exit 1; }
log()   { echo -e "  $*"; }

# ─── Cleanup on exit ────────────────────────────────────────────────────────
W1_PID=""
W2_PID=""

cleanup() {
  echo ""
  step "Cleanup"
  [ -n "$W1_PID" ] && kill "$W1_PID" 2>/dev/null && log "killed worker-01 (pid $W1_PID)" || true
  [ -n "$W2_PID" ] && kill "$W2_PID" 2>/dev/null && log "killed worker-02 (pid $W2_PID)" || true
  $DOCKER down -v --remove-orphans 2>/dev/null || true
  ok "environment torn down"
}
trap cleanup EXIT

# ─── Helpers ────────────────────────────────────────────────────────────────
api_get() { curl -sf "${BASE}${1}"; }
api_post() { curl -sf -X POST -H "Content-Type: application/json" -d "$2" "${BASE}${1}"; }

job_state() {
  api_get "/jobs/$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('State','?'))"
}

poll_state() {
  local job_id="$1"; local want="$2"; local timeout="${3:-120}"; local interval="${4:-2}"
  local deadline=$(( SECONDS + timeout ))
  while [ $SECONDS -lt $deadline ]; do
    local state
    state=$(job_state "$job_id" 2>/dev/null || echo "ERR")
    if [ "$state" = "$want" ]; then
      ok "job reached $want"
      return 0
    fi
    log "  state=$state, waiting for $want …"
    sleep "$interval"
  done
  fail "timeout: job never reached $want (last=$state)"
}

wait_for_api() {
  local deadline=$(( SECONDS + 60 ))
  log "waiting for API …"
  while [ $SECONDS -lt $deadline ]; do
    if curl -sf "$BASE/healthz" >/dev/null 2>&1; then
      ok "API is up at $BASE"
      return 0
    fi
    sleep 2
  done
  fail "API never became healthy"
}

# ─── Step 0: Reset ──────────────────────────────────────────────────────────
step "0/8  Reset environment"
$DOCKER down -v --remove-orphans 2>/dev/null || true
ok "previous stack torn down"

# ─── Step 1: Bring up postgres + forge-api (no workers) ─────────────────────
step "1/8  Start postgres + forge-api"
$DOCKER up postgres migrate forge-api ${REBUILD:+--build} -d
wait_for_api

# ─── Step 2: Build worker binary ────────────────────────────────────────────
step "2/8  Build forge-worker binary"
(cd "$REPO_ROOT" && PATH=$PATH:/usr/local/go/bin go build -o /tmp/forge-worker ./cmd/forge-worker)
ok "built → /tmp/forge-worker"

# ─── Step 3: Start worker-01 ────────────────────────────────────────────────
step "3/8  Start worker-01"
FORGE_WORKER_NAME=worker-01 \
FORGE_API_URL=$BASE \
FORGE_POLL_INTERVAL_SECS=1 \
FORGE_HEARTBEAT_INTERVAL_SECS=8 \
  /tmp/forge-worker > /tmp/forge-worker-01.log 2>&1 &
W1_PID=$!
sleep 2  # let it register and start polling
ok "worker-01 started (pid=$W1_PID)"

# ─── Step 4: Submit demo.crash-recovery job ─────────────────────────────────
step "4/8  Submit demo.crash-recovery job"
RUN_ID="demo-crash-$(date +%s)"
SUBMIT_RESP=$(api_post "/jobs" \
  "{\"kind\":\"demo.crash-recovery\",\"payload\":{\"run_id\":\"${RUN_ID}\"},\"idempotency_key\":\"${RUN_ID}\",\"max_attempts\":3,\"timeout_secs\":60}")
JOB_ID=$(echo "$SUBMIT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
ok "job_id = $JOB_ID"

# ─── Step 5: Wait for RUNNING ───────────────────────────────────────────────
step "5/8  Wait for worker-01 to reach RUNNING"
poll_state "$JOB_ID" "RUNNING" 30 1
RUNNING_AT=$(date +%s)

# ─── Step 6: Kill worker-01 (simulate crash) ────────────────────────────────
step "6/8  CRASH — killing worker-01 (pid=$W1_PID)"
kill -9 "$W1_PID" 2>/dev/null && ok "SIGKILL sent to worker-01" || warn "worker-01 was already gone"
W1_PID=""  # cleared so cleanup doesn't double-kill

# ─── Step 7: Wait for recovery (lease expiry + scheduler sweep ~40s) ────────
step "7/8  Wait for recovery scheduler to re-queue the job (~40s)"
log "  lease=30s + sweep_interval=10s → expect recovery in ~40s max"
log "  polling state …"
poll_state "$JOB_ID" "QUEUED" 90 3
RECOVERED_AT=$(date +%s)
RECOVERY_LATENCY=$(( RECOVERED_AT - RUNNING_AT ))
ok "recovery latency: ${RECOVERY_LATENCY}s"

# ─── Step 7b: Start worker-02 ───────────────────────────────────────────────
log "  starting worker-02 to pick up the recovered job"
FORGE_WORKER_NAME=worker-02 \
FORGE_API_URL=$BASE \
FORGE_POLL_INTERVAL_SECS=1 \
FORGE_HEARTBEAT_INTERVAL_SECS=8 \
  /tmp/forge-worker > /tmp/forge-worker-02.log 2>&1 &
W2_PID=$!
ok "worker-02 started (pid=$W2_PID)"

# ─── Step 8: Wait for SUCCEEDED ─────────────────────────────────────────────
step "8/8  Wait for worker-02 to succeed attempt 2"
poll_state "$JOB_ID" "SUCCEEDED" 60 2

# ─── Collect evidence ───────────────────────────────────────────────────────
step "  Collecting evidence"
mkdir -p "$EVIDENCE_DIR"
PATH=$PATH:/usr/local/go/bin go run "$REPO_ROOT/demo/collect" \
  --base-url "$BASE" \
  --job-id "$JOB_ID" \
  --out-dir "$EVIDENCE_DIR" \
  --run-id "$RUN_ID" \
  --recovery-latency-secs "$RECOVERY_LATENCY"
ok "evidence written to $EVIDENCE_DIR/"

# ─── Summary ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}═══════════════════════════════════════════════════════${RESET}"
echo -e "${GREEN}${BOLD}  FORGE Crash-Recovery Demo — COMPLETE${RESET}"
echo -e "${BOLD}═══════════════════════════════════════════════════════${RESET}"
echo ""
echo -e "  job_id           : ${CYAN}$JOB_ID${RESET}"
echo -e "  recovery latency : ${CYAN}${RECOVERY_LATENCY}s${RESET}"
echo -e "  final state      : ${GREEN}SUCCEEDED${RESET}"
echo ""
echo -e "  evidence files:"
echo -e "    ${CYAN}$EVIDENCE_DIR/raw-evidence.json${RESET}"
echo -e "    ${CYAN}$EVIDENCE_DIR/presentation.json${RESET}"
echo ""
echo -e "  worker logs:"
echo -e "    /tmp/forge-worker-01.log"
echo -e "    /tmp/forge-worker-02.log"
echo ""
echo -e "  Next: ${BOLD}make demo-video${RESET} — render the Remotion portfolio video"
echo ""
