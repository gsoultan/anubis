#!/usr/bin/env bash
# Run the whole pipeline on this machine — the same five stages CI performs,
# in the same order, with the same flags.
#
# Use it before pushing, or when CI cannot run at all: a blocked runner is not
# a reason to guess whether the tree is sound.
#
#   scripts/ci/local.sh            # everything
#   scripts/ci/local.sh --quick    # skip the backend suite and fuzz (~90s)
#
# Needs the dev database up (scripts/db.sh up). It creates and drops its own
# scratch database and never touches the dev one.
set -euo pipefail
cd "$(dirname "$0")/../.."

QUICK=0
[ "${1:-}" = "--quick" ] && QUICK=1

DEV_URL="${ANUBIS_DEV_DB_URL:-postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable}"
SCRATCH_DB="${ANUBIS_SCRATCH_DB:-anubis_localci}"
SCRATCH_URL="${DEV_URL%/*}/${SCRATCH_DB}?sslmode=disable"
CONTAINER="${ANUBIS_DB_CONTAINER:-anubis-dev-pg}"

step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
fail() { printf '\033[31mFAILED: %s\033[0m\n' "$1" >&2; exit 1; }

psql_dev() { container exec "$CONTAINER" psql -U anubis -d anubis "$@"; }

step "1/5  enforcement gates"
# The raorm drift check needs a live database; without one it skips, and a
# skipped check is not a passed check.
for s in scripts/check/*.sh; do
  ANUBIS_DB_URL="$DEV_URL" bash "$s" || fail "$(basename "$s")"
done

step "2/5  vet, build, unit tests (-race -shuffle=on)"
go vet ./... || fail "go vet"
go build ./... || fail "go build"
go test -race -shuffle=on ./... || fail "unit tests"
( cd pkg/anubis && go vet ./... && go test -race -shuffle=on ./... ) || fail "pkg/anubis"

step "3/5  console typecheck and build"
scripts/build-console.sh || fail "console"
# Leave the committed placeholder in place; only a release should replace it.
git checkout -- ui/dist/index.html 2>/dev/null || true

if [ "$QUICK" = 1 ]; then
  printf '\n\033[1m== skipped 4/5 and 5/5 (--quick)\033[0m\n'
  printf '\033[32mlocal pipeline: green (partial)\033[0m\n'
  exit 0
fi

step "4/5  integration, e2e and fuzz against a scratch database"
# DROP DATABASE fails while ANY session holds the database open — including a
# server left running from earlier work — and the failure is easy to miss.
# Check it, rather than discovering the suite ran against yesterday's rows.
psql_dev -c "DROP DATABASE IF EXISTS ${SCRATCH_DB}" | grep -qE "DROP DATABASE|does not exist" \
  || fail "could not drop ${SCRATCH_DB} — something is still connected to it"
psql_dev -c "CREATE DATABASE ${SCRATCH_DB}" >/dev/null || fail "create ${SCRATCH_DB}"
ANUBIS_DB_URL="$SCRATCH_URL" \
  ANUBIS_LOAD_WORKERS="${ANUBIS_LOAD_WORKERS:-8}" \
  ANUBIS_LOAD_BUDGET_MS="${ANUBIS_LOAD_BUDGET_MS:-2000}" \
  bash scripts/ci/backend-suite.sh || fail "backend suite"
psql_dev -c "DROP DATABASE IF EXISTS ${SCRATCH_DB}" >/dev/null || true

step "5/5  release snapshot (packaging and SBOM)"
if ! command -v goreleaser >/dev/null 2>&1; then
  echo "  goreleaser not installed — skipping (CI still covers it)"
else
  # Signing is skipped: keyless signing needs the release workflow's OIDC
  # identity and cannot work here.
  goreleaser release --snapshot --clean --skip=publish,sign >/dev/null 2>&1 || fail "goreleaser"
  bin="$(find dist -name anubisd -path '*linux_amd64*' | head -1)"
  # Count rather than `grep -q`: under `set -o pipefail`, grep -q exits on the
  # first match, strings takes SIGPIPE, and the pipeline reports failure for a
  # search that SUCCEEDED. Counting reads the whole stream.
  placeholders="$(strings "$bin" | grep -c 'Console not built' || true)"
  consoles="$(strings "$bin" | grep -c 'Anubis Console' || true)"
  [ "$placeholders" -eq 0 ] || fail "the release binary embeds the placeholder console"
  [ "$consoles" -gt 0 ] || fail "no console in the release binary"
  echo "  packaged $(ls dist | grep -cE '\.(deb|rpm|tar\.gz)$') artefacts, console embedded"
fi
git checkout -- ui/dist/index.html 2>/dev/null || true

printf '\n\033[32mlocal pipeline: green\033[0m\n'
