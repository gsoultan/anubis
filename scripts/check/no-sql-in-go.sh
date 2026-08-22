#!/usr/bin/env bash
# ADR-0009: SQL lives in db/queries/<context>/*.sql and migrations/*.sql —
# never in hand-written Go.
set -euo pipefail
cd "$(dirname "$0")/../.."
hits=$(grep -rn --include='*.go' -E '"(SELECT|INSERT INTO|UPDATE [a-z_]+ SET|DELETE FROM)[ "]' \
  cmd internal pkg 2>/dev/null \
  | grep -v '/adapter/postgres/gen/' \
  | grep -v 'internal/platform/migrate/' \
  | grep -v 'internal/scope/adapter/feed/' \
  | grep -v '_test.go' || true)
# Two documented exemptions, both in ADR-0009:
#   platform/migrate      — the hand-written runner (ADR-0002) executes SQL
#                           before the schema it would be generated against
#                           exists.
#   scope/adapter/feed    — reads FOREIGN databases (sync sources). Their
#                           schemas are unknown at build time, so sqlc cannot
#                           type-check them; identifiers are validated and
#                           quoted instead.
if [ -n "$hits" ]; then
  echo "FAIL: SQL string literals in hand-written Go:" >&2
  echo "$hits" >&2
  exit 1
fi
echo "ok: no SQL in hand-written Go"
