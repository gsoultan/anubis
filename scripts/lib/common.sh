#!/usr/bin/env bash
# Shared helpers and the single source of truth for ports.
# Sourced by every script in this directory; not executable on its own.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export ROOT

# --- port registry --------------------------------------------------------
# 7447-7449 chosen to sit clear of the usual suspects (3000/3001, 4200,
# 5000 which is also macOS AirPlay Receiver, 5173, 8000/8080/8888) and clear of
# everything already listening on this machine: 5432 Postgres, 5672/15672
# RabbitMQ, 6379 Redis, 7000, 9000/9001 MinIO. Contiguous so the whole platform
# is one memorable block.
export ANUBIS_UI_PORT="${ANUBIS_UI_PORT:-7447}"
export ANUBIS_API_PORT="${ANUBIS_API_PORT:-7448}"
export ANUBIS_DB_PORT="${ANUBIS_DB_PORT:-7449}"

export ANUBIS_DB_CONTAINER="${ANUBIS_DB_CONTAINER:-anubis-dev-pg}"
export ANUBIS_DB_IMAGE="${ANUBIS_DB_IMAGE:-docker.io/library/postgres:18-alpine}"
export ANUBIS_DB_USER="${ANUBIS_DB_USER:-anubis}"
export ANUBIS_DB_NAME="${ANUBIS_DB_NAME:-anubis}"

# --- output ---------------------------------------------------------------
if [ -t 1 ]; then
  C_DIM=$'\033[2m'; C_RED=$'\033[31m'; C_GRN=$'\033[32m'
  C_YEL=$'\033[33m'; C_BLD=$'\033[1m'; C_OFF=$'\033[0m'
else
  C_DIM=''; C_RED=''; C_GRN=''; C_YEL=''; C_BLD=''; C_OFF=''
fi
info() { printf '%s→%s %s\n' "$C_BLD" "$C_OFF" "$*"; }
ok()   { printf '%s✓%s %s\n' "$C_GRN" "$C_OFF" "$*"; }
warn() { printf '%s!%s %s\n' "$C_YEL" "$C_OFF" "$*"; }
die()  { printf '%s✗%s %s\n' "$C_RED" "$C_OFF" "$*" >&2; exit 1; }
dim()  { printf '%s%s%s\n' "$C_DIM" "$*" "$C_OFF"; }

# --- ports ----------------------------------------------------------------
port_busy() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

port_holder() {
  lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | tail -n +2 \
    | awk '{print $1" (pid "$2")"}' | sort -u | paste -sd', ' -
}

# Refuse rather than hop to the next port -- the neighbours are the API and the
# database, so a silent fallback lands the UI on top of one of them.
require_port() {
  local port="$1" what="$2"
  if port_busy "$port"; then
    die "port $port ($what) is in use by: $(port_holder "$port")
  override with: ANUBIS_${3}_PORT=<port> $0"
  fi
}

# --- containers -----------------------------------------------------------
container_exists() { container ls -a --format json 2>/dev/null | grep -q "\"$1\""; }

container_running() {
  container ls --format json 2>/dev/null \
    | python3 -c "
import sys,json
try: d=json.load(sys.stdin)
except Exception: sys.exit(1)
for c in d:
    cid=(c.get('configuration') or {}).get('id') or c.get('id')
    if cid=='$1': sys.exit(0)
sys.exit(1)"
}

psql_db() { container exec -i "$ANUBIS_DB_CONTAINER" psql -U "$ANUBIS_DB_USER" -d "$ANUBIS_DB_NAME" "$@"; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }
