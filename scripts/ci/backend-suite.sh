#!/usr/bin/env bash
# CI backend suite against a FRESH scratch database: migrate -> bootstrap ->
# serve -> integration + e2e + fuzz smoke. Locally reproducible:
#
#   container exec anubis-dev-pg psql -U anubis -c "CREATE DATABASE anubis_ci"
#   ANUBIS_DB_URL="postgres://anubis:anubis@localhost:7449/anubis_ci?sslmode=disable" \
#     scripts/ci/backend-suite.sh
#
# The suite provisions everything it needs; it never touches the dev database.
set -euo pipefail
cd "$(dirname "$0")/../.."

: "${ANUBIS_DB_URL:?set ANUBIS_DB_URL to a scratch database — the suite owns it}"
PORT="${ANUBIS_E2E_PORT:-7450}"
export ANUBIS_E2E_BASE_URL="http://localhost:${PORT}"

go run ./cmd/anubisd migrate

# Two accounts, two populations (ADR-0011): the person the person-plane tests
# sign in as, and the platform owner the admin-plane tests operate as.
go run ./cmd/anubisd bootstrap \
  --tenant impack --name Impack \
  --admin-user admin --admin-pass anubis-dev-password \
  --platform-user devadmin --platform-pass anubis-dev-password

BIN="$(mktemp -d)/anubisd"
go build -o "$BIN" ./cmd/anubisd
# The issuer is what page URLs and token `iss` claims are built from; it must
# point at THIS instance or the suite would probe whatever else is on :7448.
ANUBIS_LISTEN=":${PORT}" ANUBIS_ISSUER="${ANUBIS_E2E_BASE_URL}" "$BIN" serve &
SERVER=$!
trap 'kill "$SERVER" 2>/dev/null || true' EXIT

for i in $(seq 1 60); do
  if curl -fsS "${ANUBIS_E2E_BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "FAIL: server never became healthy on :${PORT}" >&2
    exit 1
  fi
  sleep 1
done

# -count=1: these suites hit a live server and database — external state the
# test cache cannot see, so a cached "ok" would be a lie.
go test -count=1 -tags integration ./test/integration/
go test -count=1 -tags integration ./test/e2e/

# Fuzz smoke: seconds per target, enough to catch a corpus regression. The
# path-normalisation fuzzer has found real bypasses before; it stays.
go test -run '^$' -fuzz '^FuzzNormalizePath$' -fuzztime 30s ./internal/gate/routepath
go test -run '^$' -fuzz '^FuzzOpen$' -fuzztime 20s ./internal/platform/crypto/localtoken
(cd pkg/anubis && go test -run '^$' -fuzz '^FuzzVerify$' -fuzztime 20s ./paseto)

echo "backend suite: all green"
