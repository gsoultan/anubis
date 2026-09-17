#!/usr/bin/env bash
# Generated code is committed; regeneration must be a no-op. Drift means a
# .proto, rmodel or rquery changed without regenerating (or vice versa).
#
# sqlc is gone: every context generates through storm now, and the storm half
# of this check needs a live database because stormgen PREPAREs every rquery
# declaration against it — that check IS the point.
set -euo pipefail
cd "$(dirname "$0")/../.."
export PATH="$(go env GOPATH)/bin:$PATH"
buf generate
if ! git diff --exit-code --quiet gen; then
  echo "FAIL: proto generated code drifted — run scripts/gen.sh and commit" >&2
  git --no-pager diff --stat gen >&2
  exit 1
fi

# Full paths, not a context name plus a template: the technical context's
# generated package lives under internal/platform/database, not under an
# adapter/postgres it does not have.
storm_outs=(
  internal/authz/adapter/postgres/rgen
  internal/audit/adapter/postgres/rgen
  internal/auth/adapter/postgres/rgen
  internal/control/adapter/postgres/rgen
  internal/gate/adapter/postgres/rgen
  internal/identity/adapter/postgres/rgen
  internal/scope/adapter/postgres/rgen
  internal/tenancy/adapter/postgres/rgen
  internal/platform/database/rgen
)

# Locally and in the backend suite ANUBIS_DB_URL is set; without one, say so
# instead of pretending the check ran.
if [ -n "${ANUBIS_DB_URL:-}" ]; then
  for out in "${storm_outs[@]}"; do
    go run ./cmd/stormgen generate "$out" -raw-schema live -dsn "$ANUBIS_DB_URL" >/dev/null
  done
  if ! git diff --exit-code --quiet "${storm_outs[@]}"; then
    echo "FAIL: storm generated code drifted — regenerate and commit (see cmd/stormgen)" >&2
    git --no-pager diff --stat "${storm_outs[@]}" >&2
    exit 1
  fi
  echo "ok: generated code matches sources (buf, storm)"
else
  echo "ok: generated code matches sources (buf; storm skipped — no ANUBIS_DB_URL)"
fi
