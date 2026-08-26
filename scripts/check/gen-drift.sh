#!/usr/bin/env bash
# Generated code is committed; regeneration must be a no-op. Drift means a
# .proto, .sql, rmodel or rquery changed without regenerating (or vice versa).
set -euo pipefail
cd "$(dirname "$0")/../.."
export PATH="$(go env GOPATH)/bin:$PATH"
buf generate
sqlc generate
if ! git diff --exit-code --quiet gen internal/*/adapter/postgres/gen; then
  echo "FAIL: generated code drifted — run scripts/gen.sh and commit" >&2
  git --no-pager diff --stat gen internal/*/adapter/postgres/gen >&2
  exit 1
fi
# raorm's rgen needs a live dev database (raormgen PREPAREs every rquery
# declaration against it — that check IS the point). Locally and in the
# backend suite ANUBIS_DB_URL is set; without one, say so instead of
# pretending the check ran.
if [ -n "${ANUBIS_DB_URL:-}" ]; then
  go run ./cmd/raormgen generate internal/authz/adapter/postgres/rgen \
  -raw-schema live -dsn "$ANUBIS_DB_URL" >/dev/null
  if ! git diff --exit-code --quiet internal/*/adapter/postgres/rgen; then
    echo "FAIL: raorm generated code drifted — regenerate and commit (see cmd/raormgen)" >&2
    git --no-pager diff --stat internal/*/adapter/postgres/rgen >&2
    exit 1
  fi
  echo "ok: generated code matches sources (sqlc, buf, raorm)"
else
  echo "ok: generated code matches sources (sqlc, buf; raorm skipped — no ANUBIS_DB_URL)"
fi
