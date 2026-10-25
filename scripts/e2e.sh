#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

E2E_DB="${E2E_DB:-ifter_e2e}"
E2E_PORT="${E2E_PORT:-8081}"
E2E_ADMIN_USER="${E2E_ADMIN_USER:-e2e-admin}"
E2E_ADMIN_PASSWORD="${E2E_ADMIN_PASSWORD:-E2eAdminPass123!}"
# External database overrides. When E2E_PG_CONTAINER is set, the script uses an
# already-running Postgres container instead of `docker compose up postgres`
# (useful when host port 5432 is taken by another project).
E2E_PG_HOST="${E2E_PG_HOST:-localhost}"
E2E_PG_PORT="${E2E_PG_PORT:-5432}"
E2E_PG_USER="${E2E_PG_USER:-ifter}"
E2E_PG_PASSWORD="${E2E_PG_PASSWORD:-ifter}"
E2E_PG_CONTAINER="${E2E_PG_CONTAINER:-}"
DATABASE_URL="postgres://${E2E_PG_USER}:${E2E_PG_PASSWORD}@${E2E_PG_HOST}:${E2E_PG_PORT}/${E2E_DB}?sslmode=disable"
SERVER_LOG="${E2E_SERVER_LOG:-/tmp/opencode/ifter-e2e-server.log}"
API_BIN="${E2E_API_BIN:-/tmp/opencode/ifter-e2e-api}"
IMPORTS_DIR="${E2E_IMPORTS_DIR:-/tmp/opencode/ifter-e2e-imports}"

if [ ! -f "$ROOT/e2e/node_modules/e2e/package.json" ]; then
  echo "e2e dependencies are missing. Run: cd e2e && npm install" >&2
  exit 1
fi

server_pid=""
cleanup() {
  if [ -n "$server_pid" ] && kill -0 "$server_pid" 2>/dev/null; then
    kill -INT "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

if [ -n "$E2E_PG_CONTAINER" ]; then
  for _ in $(seq 1 30); do
    if docker exec "$E2E_PG_CONTAINER" pg_isready -U "$E2E_PG_USER" -d postgres >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  docker exec "$E2E_PG_CONTAINER" psql -U "$E2E_PG_USER" -d postgres -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE IF EXISTS \"${E2E_DB}\" WITH (FORCE);" \
    -c "CREATE DATABASE \"${E2E_DB}\";" >/dev/null
else
  docker compose up -d postgres >/dev/null
  for _ in $(seq 1 30); do
    if docker compose exec -T postgres pg_isready -U "$E2E_PG_USER" -d postgres >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  docker compose exec -T postgres psql -U "$E2E_PG_USER" -d postgres -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE IF EXISTS \"${E2E_DB}\" WITH (FORCE);" \
    -c "CREATE DATABASE \"${E2E_DB}\";" >/dev/null
fi

if command -v migrate >/dev/null 2>&1; then
  migrate -path db/migrations -database "$DATABASE_URL" up
else
  go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1 \
    -path db/migrations -database "$DATABASE_URL" up
fi

rm -rf "$IMPORTS_DIR"
go build -trimpath -o "$API_BIN" ./cmd/api

IFTER_APP_ENV=development \
IFTER_HTTP_HOST=127.0.0.1 \
IFTER_HTTP_PORT="$E2E_PORT" \
IFTER_DB_HOST="$E2E_PG_HOST" \
IFTER_DB_PORT="$E2E_PG_PORT" \
IFTER_DB_USER="$E2E_PG_USER" \
IFTER_DB_PASSWORD="$E2E_PG_PASSWORD" \
IFTER_DB_NAME="$E2E_DB" \
IFTER_DB_SSLMODE=disable \
IFTER_LOG_LEVEL=warn \
IFTER_LOG_FORMAT=text \
IFTER_AUTH_JWT_SECRET="e2e-secret-at-least-32-characters-long" \
IFTER_AUTH_BCRYPT_COST=10 \
IFTER_AUTH_LOGIN_RATE_LIMIT=1000 \
IFTER_AUTH_LOGIN_RATE_WINDOW=1m \
IFTER_AUTH_BOOTSTRAP_ADMIN_USERNAME="$E2E_ADMIN_USER" \
IFTER_AUTH_BOOTSTRAP_ADMIN_EMAIL="e2e-admin@example.test" \
IFTER_AUTH_BOOTSTRAP_ADMIN_PASSWORD="$E2E_ADMIN_PASSWORD" \
IFTER_AUTH_BOOTSTRAP_ADMIN_FULL_NAME="E2E Administrator" \
IFTER_IMPORTS_DIR="$IMPORTS_DIR" \
"$API_BIN" >"$SERVER_LOG" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${E2E_PORT}/readyz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "e2e API exited before becoming ready" >&2
    cat "$SERVER_LOG" >&2 || true
    exit 1
  fi
  sleep 1
done

if [ "$ready" -ne 1 ]; then
  echo "e2e API did not become ready on port ${E2E_PORT}" >&2
  cat "$SERVER_LOG" >&2 || true
  exit 1
fi

set +e
(
  cd "$ROOT/e2e"
  API_URL="http://127.0.0.1:${E2E_PORT}" E2E_TELEMETRY_DISABLED=1 npx e2e run "$@"
)
status=$?
set -e

if [ "$status" -ne 0 ]; then
  echo "--- e2e API log ---" >&2
  tail -n 100 "$SERVER_LOG" >&2 || true
fi
exit "$status"
