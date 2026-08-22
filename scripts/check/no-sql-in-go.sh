#!/usr/bin/env bash
# ADR-0009: SQL lives in db/queries/*.sql and migrations/*.sql — never in
# hand-written Go. Generated code (internal/adapter/postgres/gen) is exempt.
set -euo pipefail
cd "$(dirname "$0")/../.."
hits=$(grep -rn --include='*.go' -E '"(SELECT|INSERT INTO|UPDATE [a-z_]+ SET|DELETE FROM)[ "]' \
  cmd internal pkg 2>/dev/null \
  | grep -v 'internal/adapter/postgres/gen/' \
  | grep -v 'internal/migrate/' \
  | grep -v 'internal/repository/feed/' \
  | grep -v '_test.go' || true)
# Two documented exemptions, both in ADR-0009:
#   internal/migrate       — the hand-written runner (ADR-0002) must execute
#                            SQL before any schema exists to query.
#   internal/repository/feed — reads FOREIGN databases (scope sync sources).
#                            Their schemas are unknown at build time, so sqlc
#                            cannot type-check them; identifiers are validated
#                            and quoted instead. Nothing here touches Anubis's
#                            own schema.
if [ -n "$hits" ]; then
  echo "FAIL: SQL string literals in hand-written Go:" >&2
  echo "$hits" >&2
  exit 1
fi
echo "ok: no SQL in hand-written Go"
