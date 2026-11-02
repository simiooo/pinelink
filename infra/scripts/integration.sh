#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
HOST=${PINELINK_MIDDLEWARE_HOST:-127.0.0.1}
HTTP_ADDR=${PINELINK_TEST_HTTP_ADDR:-127.0.0.1:18080}
AUTH_HTTP_ADDR=${PINELINK_TEST_AUTH_HTTP_ADDR:-127.0.0.1:18081}
export PINELINK_DATABASE_URL=${PINELINK_DATABASE_URL:-"postgres://pinelink:pinelink_dev@${HOST}:55432/pinelink?sslmode=disable"}
export PINELINK_NATS_URL=${PINELINK_NATS_URL:-"nats://${HOST}:54222"}
export PINELINK_S3_ENDPOINT=${PINELINK_S3_ENDPOINT:-"${HOST}:59000"}
export PINELINK_S3_ACCESS_KEY=${PINELINK_S3_ACCESS_KEY:-pinelink}
export PINELINK_S3_SECRET_KEY=${PINELINK_S3_SECRET_KEY:-pinelink_dev_secret}
export PINELINK_S3_BUCKET=${PINELINK_S3_BUCKET:-pinelink-dev}
export PINELINK_S3_USE_SSL=${PINELINK_S3_USE_SSL:-false}
export PINELINK_SERVICE_TEST_DATABASE_URL="$PINELINK_DATABASE_URL"
export PINELINK_MIGRATIONS_DIR="$ROOT/infra/migrations"
export PINELINK_HTTP_ADDR="$HTTP_ADDR"

cd "$ROOT"
docker compose -f infra/compose.yaml up -d --wait

TMP=$(mktemp -d)
BACKEND_PID=""
AUTH_BACKEND_PID=""
cleanup() {
  status=$?
  if [[ $status -ne 0 ]]; then
    [[ -f "$TMP/backend.log" ]] && cat "$TMP/backend.log"
    [[ -f "$TMP/auth-backend.log" ]] && cat "$TMP/auth-backend.log"
  fi
  if [[ -n "$BACKEND_PID" ]]; then
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  if [[ -n "$AUTH_BACKEND_PID" ]]; then
    kill "$AUTH_BACKEND_PID" 2>/dev/null || true
    wait "$AUTH_BACKEND_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

EXE=""
if [[ "$(go env GOOS)" == "windows" ]]; then
  EXE=".exe"
fi
(cd backend && go test ./... && go build -o "$TMP/pinelinkd$EXE" ./cmd/pinelinkd)
(cd cli && go test ./... && go build -o "$TMP/pinelink$EXE" ./cmd/pinelink)

"$TMP/pinelinkd$EXE" >"$TMP/backend.log" 2>&1 &
BACKEND_PID=$!
for _ in $(seq 1 60); do
  if curl -fsS "http://${HTTP_ADDR}/healthz" >/dev/null; then
    break
  fi
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    cat "$TMP/backend.log"
    exit 1
  fi
  sleep 1
done
curl -fsS "http://${HTTP_ADDR}/healthz" >/dev/null

PINELINK_HTTP_ADDR="$AUTH_HTTP_ADDR" PINELINK_DEVICE_AUTH_REQUIRED=true "$TMP/pinelinkd$EXE" >"$TMP/auth-backend.log" 2>&1 &
AUTH_BACKEND_PID=$!
for _ in $(seq 1 60); do
  if curl -fsS "http://${AUTH_HTTP_ADDR}/healthz" >/dev/null; then
    break
  fi
  if ! kill -0 "$AUTH_BACKEND_PID" 2>/dev/null; then
    cat "$TMP/auth-backend.log"
    exit 1
  fi
  sleep 1
done
curl -fsS "http://${AUTH_HTTP_ADDR}/healthz" >/dev/null

export PINELINK_TEST_SERVER="http://${HTTP_ADDR}"
export PINELINK_TEST_AUTH_SERVER="http://${AUTH_HTTP_ADDR}"
export PINELINK_TEST_CLI="$TMP/pinelink$EXE"
(cd infra && go test -v ./integration -run "${PINELINK_TEST_FILTER:-.}")
