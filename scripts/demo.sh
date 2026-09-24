#!/usr/bin/env bash
# FORGE demo: submit jobs, verify idempotency, poll to completion, show metrics.
# Requires the API to be running: make run
set -euo pipefail

BASE="${FORGE_API_URL:-http://localhost:8080}"
RUN_ID="demo-$(date +%s)"

echo "=== FORGE Demo ==="
echo ""

# 1. Health check
echo "1. Health check"
curl -sf "$BASE/healthz" | python3 -m json.tool
echo ""

# 2. Submit 5 jobs (idempotency_key required by the API)
echo "2. Submitting 5 jobs..."
JOB_IDS=()
for i in $(seq 1 5); do
  RESP=$(curl -sf -X POST "$BASE/jobs" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"demo.task\",\"payload\":{\"index\":$i},\"idempotency_key\":\"${RUN_ID}-job-${i}\",\"max_attempts\":3}")
  ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
  JOB_IDS+=("$ID")
  echo "  Submitted job $i: $ID"
done
echo ""

# 3. Idempotency test — same idempotency_key must return the same job
echo "3. Idempotency — submitting same idempotency_key twice..."
IK_KEY="${RUN_ID}-idem-001"
IK_RESP1=$(curl -sf -X POST "$BASE/jobs" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"demo.idempotent\",\"payload\":{},\"idempotency_key\":\"${IK_KEY}\",\"max_attempts\":1}")
IK_RESP2=$(curl -sf -X POST "$BASE/jobs" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"demo.idempotent\",\"payload\":{},\"idempotency_key\":\"${IK_KEY}\",\"max_attempts\":1}")
ID1=$(echo "$IK_RESP1" | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
ID2=$(echo "$IK_RESP2" | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")
if [ "$ID1" = "$ID2" ]; then
  echo "  PASS: both calls returned the same job ID ($ID1)"
else
  echo "  FAIL: got different IDs: $ID1 vs $ID2"
  exit 1
fi
echo ""

# 4. Poll for completion (State field is PascalCase in GET responses)
echo "4. Waiting for jobs to complete (up to 60s)..."
DEADLINE=$((SECONDS + 60))
while [ $SECONDS -lt $DEADLINE ]; do
  DONE=0
  for ID in "${JOB_IDS[@]}"; do
    STATE=$(curl -sf "$BASE/jobs/$ID" | python3 -c "import sys,json; print(json.load(sys.stdin)['State'])")
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
  STATE=$(curl -sf "$BASE/jobs/$ID" | python3 -c "import sys,json; print(json.load(sys.stdin)['State'])")
  echo "  $ID  $STATE"
done
echo ""

# 6. Event trail for the first job
FIRST="${JOB_IDS[0]}"
echo "6. Event trail for job $FIRST:"
curl -sf "$BASE/jobs/$FIRST/events" | python3 -c "
import sys, json
events = json.load(sys.stdin)
for e in events:
    ts = e['OccurredAt'][:19]
    typ = e['Type']
    frm = e.get('FromState') or '-'
    to  = e.get('ToState')  or '-'
    print(f'  {ts}  {typ:12}  {frm:10} -> {to}')
"
echo ""

# 7. Active workers
echo "7. Active workers:"
curl -sf "$BASE/workers" | python3 -c "
import sys, json
workers = json.load(sys.stdin)
for w in workers:
    print(f'  {w[\"ID\"]}  {w[\"Name\"]:20}  {w[\"State\"]}')
if not workers:
    print('  (none registered yet)')
"
echo ""

# 8. Prometheus metrics
echo "8. Metrics (forge_ instruments):"
curl -sf "$BASE/metrics" | grep "^forge_[a-z]" | grep -v "^#" | head -20
echo ""

echo "=== Demo complete ==="
