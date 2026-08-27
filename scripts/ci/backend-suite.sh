#!/usr/bin/env bash
# CI backend suite against a FRESH scratch database: migrate -> bootstrap ->
# serve -> integration + e2e + fuzz smoke. Locally reproducible:
#
#   container exec anubis-dev-pg psql -U anubis -c "CREATE DATABASE anubis_ci"
#   ANUBIS_DB_URL="postgres://anubis:anubis@localhost:7449/anubis_ci?sslmode=disable" \
#     scripts/ci/backend-suite.sh
#
# The suite provisions everything it needs; it never touches the dev database.
#
# The scratch database must be genuinely EMPTY. `DROP DATABASE` fails while
# anything still holds a connection to it — a stray server, a psql session —
# and a suite run against leftover rows reports failures that are nothing to
# do with the code. Check that the DROP actually printed DROP DATABASE.
set -euo pipefail
cd "$(dirname "$0")/../.."

: "${ANUBIS_DB_URL:?set ANUBIS_DB_URL to a scratch database — the suite owns it}"
PORT="${ANUBIS_E2E_PORT:-7450}"
export ANUBIS_E2E_BASE_URL="http://localhost:${PORT}"

go run ./cmd/anubisd migrate

# The scratch database is now EXACTLY what migrations produce — the strongest
# schema stormgen can PREPARE the rquery declarations against. A declaration
# that drifted from migrations, or generated output that drifted from a
# declaration, fails here naming the statement.
go run ./cmd/stormgen generate internal/authz/adapter/postgres/rgen \
  -raw-schema live -dsn "$ANUBIS_DB_URL" >/dev/null
if ! git diff --exit-code --quiet internal/*/adapter/postgres/rgen; then
  echo "FAIL: storm generated code drifted — regenerate and commit (see cmd/stormgen)" >&2
  git --no-pager diff --stat internal/*/adapter/postgres/rgen >&2
  exit 1
fi

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
# The scope-sync e2e tests serve their feeds from httptest on loopback, which
# the egress policy refuses by default: in production a feed pointed at
# 127.0.0.1 is a feed pointed at Anubis itself. The suite opts in the same way
# a developer with a local source would.
ANUBIS_SYNC_ALLOW_LOOPBACK=1 \
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
#
# -parallel is bounded on purpose. Go fuzzes with one worker per CPU by
# default, and on a 2-core shared runner — or a laptop already running this
# suite's server and database — oversubscription makes a worker time out and
# the step fail with no crasher written. That is a flaky pipeline reporting a
# security finding it did not make. Four workers still execute millions of
# inputs per target.
FUZZ_PAR="${ANUBIS_FUZZ_PARALLEL:-4}"
go test -run '^$' -fuzz '^FuzzNormalizePath$' -fuzztime 30s -parallel "$FUZZ_PAR" ./internal/gate/routepath
go test -run '^$' -fuzz '^FuzzOpen$' -fuzztime 20s -parallel "$FUZZ_PAR" ./internal/platform/crypto/localtoken
(cd pkg/anubis && go test -run '^$' -fuzz '^FuzzVerify$' -fuzztime 20s -parallel "$FUZZ_PAR" ./paseto)

echo "backend suite: all green"
