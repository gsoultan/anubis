#!/usr/bin/env bash
# Record one soak reading for the storm migration (storm docs/PRODUCTION-READINESS.md
# P3, window 2026-08-25 → 2026-09-08).
#
#   ANUBIS_DB_URL=... scripts/soak-record.sh
#
# A soak that only proves "no crash" proves nothing, so this collects the four
# signals that would actually move the v0.1.0 tag, appends them to
# docs/soak-storm.md, and says plainly when one could not be read rather than
# recording a zero.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${ANUBIS_DB_URL:?set ANUBIS_DB_URL — the soak reads the live dev database}"
OUT="docs/soak-storm.md"
STAMP="$(date -u '+%Y-%m-%d %H:%M UTC')"

# 1+2. Latency through pgx and through the storm repository, plus the shape
# counts, all from the integration suite that already measures them.
LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT
go test -count=1 -tags integration ./test/integration/ -v \
  -run 'TestAuthorizeLatencyBudget|TestStormSlice_AuthorizeLatencyBudget|TestStormFull_VaryingValuesDoNotMintShapes' \
  > "$LOG" 2>&1 || { echo "FAIL: the budget/shape tests did not pass — that IS the finding" >&2; tail -20 "$LOG" >&2; exit 1; }

pgx_p95="$(grep -o 'authorize over pgx: .*p95=[^ ]*' "$LOG" | grep -o 'p95=[^ ]*' | cut -d= -f2 || true)"
storm_p95="$(grep -o 'authorize over storm repository: .*p95=[^ ]*' "$LOG" | grep -o 'p95=[^ ]*' | cut -d= -f2 || true)"
shapes="$(grep -o 'shapes [0-9]* → [0-9]*' "$LOG" | tail -1 || true)"
flushes="$(grep -o 'flushes [0-9]* → [0-9]*' "$LOG" | tail -1 || true)"

# 3. Resident memory of a running server, if one is running. The signal is a
# PLATEAU, so a single reading means little on its own and everything in a
# column of them.
rss="not running"
pid="$(pgrep -f 'anubisd serve' | head -1 || true)"
if [ -n "$pid" ]; then
  rss="$(ps -o rss= -p "$pid" | awk '{printf "%.1f MB", $1/1024}')"
fi

# 4. Generated code still matches its declarations and the live schema.
drift="clean"
if ! go run ./cmd/stormgen generate internal/authz/adapter/postgres/rgen \
     -raw-schema live -dsn "$ANUBIS_DB_URL" >/dev/null 2>&1; then
  drift="stormgen FAILED"
elif ! git diff --quiet -- internal/authz/adapter/postgres/rgen; then
  drift="DRIFTED"
fi

if [ ! -f "$OUT" ]; then
  cat > "$OUT" <<'HEADER'
# storm soak readings

The two-week soak gating storm's v0.1.0 tag (2026-08-25 → 2026-09-08). Record
one row per reading with `scripts/soak-record.sh`.

What would move the tag, from storm's `docs/PRODUCTION-READINESS.md` P3: an
authorize p95 past the 2 ms budget, a shape count that grows with traffic
instead of plateauing, or resident memory without a plateau. The date is not
the gate; these are.

| when | p95 via pgx | p95 via storm | shapes | flushes | anubisd RSS | rgen |
|---|---|---|---|---|---|---|
HEADER
fi

printf '| %s | %s | %s | %s | %s | %s | %s |\n' \
  "$STAMP" "${pgx_p95:-?}" "${storm_p95:-?}" "${shapes:-?}" "${flushes:-?}" "$rss" "$drift" >> "$OUT"

echo "recorded → $OUT"
tail -2 "$OUT"
