#!/usr/bin/env bash
# Give the soak something to measure.
#
#   ANUBIS_DB_URL=... scripts/soak-load.sh [rounds]
#
# scripts/soak-record.sh reads four signals, and two of them — resident memory
# and shape growth under traffic — need a process that STAYS UP and is being
# used. Every reading so far recorded "not running", which means the soak was
# proving that the test suite passes. That is not what it promised.
#
# So this starts a server, drives real request traffic through it, and records
# a row after each round. RSS across rows is the plateau signal: a cache with a
# ceiling and pools that recycle show a step and then a flat line; a leak shows
# a ramp that does not stop.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${ANUBIS_DB_URL:?set ANUBIS_DB_URL}"
ROUNDS="${1:-4}"
PORT="${ANUBIS_SOAK_PORT:-7470}"
export ANUBIS_E2E_BASE_URL="http://localhost:${PORT}"

# The server must already have a schema and an account to authenticate as;
# bootstrap is idempotent enough to re-run against a dev database.
go run ./cmd/anubisd migrate >/dev/null
go run ./cmd/anubisd bootstrap \
  --tenant impack --name Impack \
  --admin-user admin --admin-pass anubis-dev-password \
  --platform-user devadmin --platform-pass anubis-dev-password >/dev/null 2>&1 || true

BIN="$(mktemp -d)/anubisd"
go build -o "$BIN" ./cmd/anubisd
DEBUG_PORT="${ANUBIS_SOAK_DEBUG_PORT:-7471}"
ANUBIS_LISTEN=":${PORT}" ANUBIS_ISSUER="${ANUBIS_E2E_BASE_URL}" \
  ANUBIS_DEBUG_LISTEN="127.0.0.1:${DEBUG_PORT}" "$BIN" serve >/dev/null 2>&1 &
SERVER=$!
# Wait for the drain, do not just signal it. serve.go says why: "an audit
# event dropped during drain is a security record lost", and the audit writer
# is a queue with a grace period. Killing and exiting immediately leaves that
# queue unflushed.
drain() {
  kill "$SERVER" 2>/dev/null || return 0
  for _ in $(seq 1 30); do
    kill -0 "$SERVER" 2>/dev/null || return 0
    sleep 1
  done
  kill -9 "$SERVER" 2>/dev/null || true
}
trap drain EXIT

for i in $(seq 1 60); do
  curl -fsS "${ANUBIS_E2E_BASE_URL}/healthz" >/dev/null 2>&1 && break
  if [ "$i" -eq 60 ]; then echo "server never became healthy on :${PORT}" >&2; exit 1; fi
  sleep 1   # a retry loop with no delay is 60 attempts in one millisecond
done
echo "server up on :${PORT} (pid ${SERVER})"

rss_now() { ps -o rss= -p "$SERVER" | awk '{printf "%.0f", $1/1024}'; }

# RSS conflates three different things: live data, memory Go has not returned
# to the OS yet, and — in this application — a 50,000-grant snapshot rebuilt
# every 30 seconds. HeapInuse from the server's own expvar separates them:
# live data that keeps growing is retention, RSS that stays high while
# HeapInuse is flat is just the scavenger being lazy.
heap_mb() {
  curl -fsS "http://127.0.0.1:${DEBUG_PORT}/debug/vars" 2>/dev/null |
    python3 -c 'import json,sys
try:
    m = json.load(sys.stdin)["memstats"]
    print("%d %d %d" % (m["HeapInuse"]//1048576, m["HeapAlloc"]//1048576, m["NumGC"]))
except Exception:
    print("? ? ?")'
}

printf '%-22s %8s %10s %10s %6s\n' "phase" "RSS MB" "heapInuse" "heapAlloc" "GCs"
read -r hi ha gc <<< "$(heap_mb)"
printf '%-22s %8s %10s %10s %6s\n' "0 idle" "$(rss_now)" "$hi" "$ha" "$gc"
base_hi="$hi"

for round in $(seq 1 "$ROUNDS"); do
  # -v because the rate only appears in the log when the test FAILS otherwise:
  # a passing round printed "?" in the first version of this script, which
  # read as "no load happened" when it meant the opposite.
  go test -count=1 -v -tags integration ./test/e2e/ -run TestAuthorizeUnderConcurrency \
    >"/tmp/soak-round-${round}.log" 2>&1 || true
  rate="$(grep -oE '[0-9]+/s' "/tmp/soak-round-${round}.log" | head -1 || true)"
  read -r hi ha gc <<< "$(heap_mb)"
  printf '%-22s %8s %10s %10s %6s   %s\n' "${round} after load" "$(rss_now)" "$hi" "$ha" "$gc" "${rate:-?}"
done

settle="${ANUBIS_SOAK_SETTLE:-90}"
echo
echo "idling ${settle}s..."
sleep "$settle"
read -r hi ha gc <<< "$(heap_mb)"
printf '%-22s %8s %10s %10s %6s\n' "quiet" "$(rss_now)" "$hi" "$ha" "$gc"

echo
if [ "$hi" = "?" ] || [ "$base_hi" = "?" ]; then
  echo "expvar unavailable — RSS alone cannot tell retention from scavenger lag."
else
  # Deliberately a report, not a verdict. The first version of this printed
  # "returned to 450 MB against 235 MB at rest" — which read as recovery while
  # the number had in fact gone UP. A threshold that can describe a rise as a
  # return is worse than no threshold.
  echo "Neither column is 'memory used', and mixing them up is the trap:"
  echo "  · RSS can sit BELOW heapInuse. Go releases pages with MADV_FREE, so"
  echo "    the OS reclaims them while Go still counts the spans as in use."
  echo "  · heapInuse counts spans including free objects inside them, so it"
  echo "    lags real live data. heapAlloc right after a GC is the closer"
  echo "    estimate, and the GC column tells you whether one just ran."
  echo
  echo "What the series is good for, which is comparison rather than absolutes:"
  echo "  · flat across the load rounds  → the request path retains nothing"
  echo "    per request, whatever RSS does; that is the storm claim."
  echo "  · rising while QUIET           → something periodic allocates. In"
  echo "    this application that is the 50k-grant snapshot rebuilt every 30s,"
  echo "    which runs whether or not requests arrive."
  echo "  · rising WITH load and staying → retention on the request path, and"
  echo "    the thing this soak exists to catch."
  echo
  echo "idle ${base_hi} MB → under load ${hi} MB live, after ${gc} collections."
fi
echo
echo "quiet reading:"
bash scripts/soak-record.sh 2>&1 | tail -2
