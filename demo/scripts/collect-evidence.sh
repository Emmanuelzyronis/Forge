#!/usr/bin/env bash
# Collect evidence for an already-completed demo job.
# Usage: bash demo/scripts/collect-evidence.sh <job-id> [base-url]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
JOB_ID="${1:?Usage: $0 <job-id> [base-url]}"
BASE="${2:-http://localhost:8081}"
EVIDENCE_DIR="$REPO_ROOT/demo/evidence"

mkdir -p "$EVIDENCE_DIR"
PATH=$PATH:/usr/local/go/bin go run "$REPO_ROOT/demo/collect" \
  --base-url "$BASE" \
  --job-id "$JOB_ID" \
  --out-dir "$EVIDENCE_DIR"

echo "Evidence written to $EVIDENCE_DIR/"
