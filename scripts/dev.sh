#!/usr/bin/env bash
# Bring up the whole Anubis dev environment.
#
#   scripts/dev.sh              database + api + console, seeded and signable-into
#   scripts/dev.sh --no-db      skip the database
#   scripts/dev.sh --ui-only    console only
#
# Ctrl-C stops everything started here. The database container is deliberately
# left running: it holds the seeded dataset, and restarting it on every session
# would mean reseeding 150k grants for no reason.
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

WITH_DB=1; WITH_API=1
for arg in "$@"; do
  case "$arg" in
    --no-db)   WITH_DB=0 ;;
    --ui-only) WITH_DB=0; WITH_API=0 ;;
    -h|--help) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)         die "unknown flag '$arg'" ;;
  esac
done

PIDS=()
cleanup() {
  [ ${#PIDS[@]} -gt 0 ] && kill "${PIDS[@]}" 2>/dev/null || true
  printf '\n'; dim "stopped (database left running — scripts/db.sh down to stop it)"
}
trap cleanup EXIT INT TERM

printf '%s' "$C_BLD"
cat <<'BANNER'
  ___  _ __  _   _ | |__  (_) ___
 / _ \| '_ \| | | || '_ \ | |/ __|
| (_| | | | | |_| || |_) || |\__ \
 \__,_|_| |_|\__,_||_.__/ |_||___/
BANNER
printf '%s' "$C_OFF"
dim "  console :$ANUBIS_UI_PORT   api :$ANUBIS_API_PORT   database :$ANUBIS_DB_PORT"
echo

# seed does up + migrate + dataset + an account that can sign in. Without the
# last of those the console comes up against a database holding fifty thousand
# people and no way to authenticate as any of them.
[ "$WITH_DB" = 1 ] && "$ROOT/scripts/db.sh" seed

if [ "$WITH_API" = 1 ]; then
  require_port "$ANUBIS_API_PORT" "api" "API"
  "$ROOT/scripts/api.sh" & PIDS+=($!)
fi

# The console runs in the foreground so its output is the one you watch and
# Ctrl-C reaches it directly.
exec "$ROOT/scripts/ui.sh"
