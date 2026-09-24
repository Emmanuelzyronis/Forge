#!/bin/sh
# Entrypoint for Railway (and any container host).
# 1. Map Railway's injected DATABASE_URL → FORGE_DATABASE_URL if not explicitly set.
# 2. Map Railway's injected PORT → FORGE_LISTEN_ADDR.
# 3. Run database migrations.
# 4. Start forge-api.
set -e

export FORGE_DATABASE_URL="${FORGE_DATABASE_URL:-${DATABASE_URL:-}}"
export FORGE_LISTEN_ADDR=":${PORT:-8080}"

if [ -z "$FORGE_DATABASE_URL" ]; then
  echo "start.sh: FORGE_DATABASE_URL (or DATABASE_URL) must be set" >&2
  exit 1
fi

echo "start.sh: running migrations"
migrate -path /app/migrations -database "$FORGE_DATABASE_URL" up

echo "start.sh: starting forge-api on $FORGE_LISTEN_ADDR"
exec /app/forge-api
