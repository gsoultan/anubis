#!/usr/bin/env bash
# Full reproducible rebuild: schema -> seed -> correctness -> external
# populations -> negative -> performance.
set -uo pipefail
C=anubis-dev-pg
psql() { container exec -i "$C" psql -U anubis -d anubis "$@"; }
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

echo "==> dropping and rebuilding schema"
psql -qc "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" >/dev/null 2>&1
for f in migrations/*.sql; do
  if ! psql -v ON_ERROR_STOP=1 -q < "$f" >"$TMP/m.log" 2>&1; then
    echo "FAILED: $f"; cat "$TMP/m.log"; exit 1
  fi
done
echo "    $(ls migrations/*.sql | wc -l | tr -d ' ') migrations applied"

echo "==> seeding"
if ! psql -v ON_ERROR_STOP=1 -q < bench/seed.sql >"$TMP/s.log" 2>&1; then
  echo "SEED FAILED"; grep -i error "$TMP/s.log" | head -3; exit 1
fi
psql -tAc "SELECT '    '||count(*)||' grants, '||
  (SELECT count(*) FROM grant_scopes)||' grant_scopes, '||
  (SELECT count(*) FROM scope_closure)||' closure rows, '||
  (SELECT count(*) FROM identities)||' identities across '||
  (SELECT count(*) FROM realms)||' realms' FROM grants;"

echo "==> correctness"
psql < bench/run.sql 2>&1 | grep -E '\->' | sed 's/^/    /'

echo "==> external populations (suppliers, applicants)"
# ONE invocation: the file is not idempotent (it inserts roles), and running it
# twice previously masked the escalation-guard count behind a duplicate-key error.
psql < bench/realms.sql >"$TMP/realms.log" 2>&1
grep -E '\->' "$TMP/realms.log" | grep -v ERROR | sed 's/^/    /'
grep -E 'same username' "$TMP/realms.log" | sed 's/^/    /'
echo "    $(grep -c '^ERROR' "$TMP/realms.log")/3 escalation attempts rejected"

echo "==> memberships (assign fan-out, unassign, guard)"
psql < bench/memberships.sql >"$TMP/mem.log" 2>&1
grep -E 'assign|member sees|after unassign' "$TMP/mem.log" | sed 's/^ */    /'
echo "    $(grep -c '^ERROR' "$TMP/mem.log")/1 wrong-population assignment rejected"

echo "==> scope sync (reconcile by external_ref)"
psql < bench/sync.sql 2>&1 | grep -E 'apply:|renamed applied|archived node|rerun-dry' | sed 's/^ */    /'

echo "==> negative (all must be blocked by the schema)"
psql < bench/negative.sql >"$TMP/neg.log" 2>&1
# Per CASE, not per ERROR line. The count this replaced was "N/9" against a
# file that had grown to twenty cases, so it read 21/9 and nobody could tell
# which case had stopped being checked -- and the failure it has to catch is
# exactly one case going quiet. That happens without any guard breaking: case
# 13b matched zero rows on a database with no memberships in it, inserted
# nothing, committed cleanly and printed nothing. A negative test that passes
# by doing nothing looks identical to one that passes by being enforced.
# 8c legitimately errors twice on a rerun, which is why this counts cases
# with no error rather than errors.
unguarded=$(awk '/^--- /{if (c != "" && n == 0) print c; c=$0; n=0}
                 /^ERROR/{n++}
                 END{if (c != "" && n == 0) print c}' "$TMP/neg.log")
cases=$(grep -c '^--- ' "$TMP/neg.log")
if [ -n "$unguarded" ]; then
  echo "    FAIL: illegal writes that were NOT rejected:"
  echo "$unguarded" | sed 's/^/      /'
else
  echo "    $cases/$cases illegal writes rejected"
fi

echo "==> performance"
psql < bench/final.sql 2>&1 | grep -E "^Time:" | head -1 | sed 's/^/    20k decisions: /'
