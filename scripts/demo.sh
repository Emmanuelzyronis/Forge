#!/usr/bin/env bash
# FORGE demo: submit jobs, verify idempotency, poll to completion, show metrics.
# Requires the API to be running: make run
set -euo pipefail

BASE="${FORGE_API_URL:-http://localhost:8080}"

echo "=== FORGE Demo ==="
echo ""

# 1. Health check
echo "1. Health check"
curl -sf "$BASE/healthz" | python3 -m json.tool
echo ""

# 2. Submit 5 jobs
echo "2. Submitting 5 jobs..."
JOB_IDS=()
for i in $(seq 1 5); do
  RESP=$(curl -sf -X POST "$BASE/jobs" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"demo.task\",\"payload\":{\"index\":$i},\"max_attempts\":3}")
  ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
  JOB_IDS+=("$ID")
  echo "  Submitted job $i: $ID"
done
echo ""

# 3. Idempotency test
echo "3. Idempotency — submitting same idempotency_key twice..."
IK_RESP1=$(curl -sf -X POST "$BASE/jobs" \
  -H "Content-Type: application/json" \
  -d '{"kind":"demo.idempotent","payload":{},"idempotency_key":"demo-idem-001","max_attempts":1}')
IK_RESP2=$(curl -sf -X POST "$BASE/jobs" \
  -H "Content-Type: application/json" \
  -d '{"kind":"demo.idempotent","payload":{},"idempotency_key":"demo-idem-001","max_attempts":1}')
ID1=$(echo "$IK_RESP1" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
ID2=$(echo "$IK_RESP2" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
if [ "$ID1" = "$ID2" ]; then
  echo "  PASS: both calls returned the same job ID ($ID1)"
else
  echo "  FAIL: got different IDs: $ID1 vs $ID2"
  exit 1
fi
echo ""

# 4. Poll for completion
echo "4. Waiting for jobs to complete (up to 60s)..."
DEADLINE=$((SECONDS + 60))
while [ $SECONDS -lt $DEADLINE ]; do
  DONE=0
  for ID in "${JOB_IDS[@]}"; do
    STATE=$(curl -sf "$BASE/jobs/$ID" | python3 -c "import sys,json; print(json.load(sys.stdin)['state'])")
    if [ "$STATE" = "SUCCEEDED" ] || [ "$STATE" = "FAILED" ]; then
      DONE=$((DONE + 1))
    fi
  done
  if [ "$DONE" -eq "${#JOB_IDS[@]}" ]; then
    echo "  All ${#JOB_IDS[@]} jobs completed."
    break
  fi
  echo "  $DONE/${#JOB_IDS[@]} done, waiting..."
  sleep 3
done
echo ""

# 5. Final states
echo "5. Final job states:"
for ID in "${JOB_IDS[@]}"; do
  STATE=$(curl -sf "$BASE/jobs/$ID" | python3 -c "import sys,json; print(json.load(sys.stdin)['state'])")
  echo "  $ID  $STATE"
done
echo ""

# 6. Event history for first job
FIRST="${JOB_IDS[0]}"
echo "6. Event trail for job $FIRST:"
curl -sf "$BASE/jobs/$FIRST/events" | python3 -c "
import sys, json
events = json.load(sys.stdin)
for e in events:
    print(f'  {e[\"occurred_at\"][:19]}  {e[\"type\"]:12}  {e.get(\"from_state\") or \"-\":8} -> {e.get(\"to_state\") or \"-\"}')
"
echo ""

# 7. Active workers
echo "7. Active workers:"
curl -sf "$BASE/workers" | python3 -c "
import sys, json
workers = json.load(sys.stdin)
for w in workers:
    print(f'  {w[\"id\"]}  {w[\"name\"]:20}  {w[\"state\"]}')
"
echo ""

# 8. Metrics sample
echo "8. Metrics (forge_ instruments):"
curl -sf "$BASE/metrics" | grep "^forge_[a-z]" | grep -v "^#" | head -20
echo ""

echo "=== Demo complete ==="
