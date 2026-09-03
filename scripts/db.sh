#!/usr/bin/env bash
# Database lifecycle for the Anubis dev environment.
#
#   scripts/db.sh up          start (creating it if absent)
#   scripts/db.sh down        stop, keeping data
#   scripts/db.sh status      state, port publishing, row counts
#   scripts/db.sh migrate     apply pending migrations
#   scripts/db.sh baseline    record existing files as applied (adopting a db)
#   scripts/db.sh seed        migrate, load the dev dataset, ensure a login
#   scripts/db.sh reset       DESTRUCTIVE: drop schema, migrate, seed, validate
#   scripts/db.sh psql [...]  interactive shell, or run arguments
#   scripts/db.sh recreate    DESTRUCTIVE: delete and rebuild the container
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
need container

create_container() {
  info "creating $ANUBIS_DB_CONTAINER (postgres 18, published on :$ANUBIS_DB_PORT)"
  # Settings are not decoration: random_page_cost suits SSD, track_io_timing is
  # required for EXPLAIN (BUFFERS) attribution, and shared_buffers keeps the hot
  # indexes resident so benchmarks measure the design, not the page cache.
  container run -d --name "$ANUBIS_DB_CONTAINER" \
    -e POSTGRES_PASSWORD="$ANUBIS_DB_USER" \
    -e POSTGRES_USER="$ANUBIS_DB_USER" \
    -e POSTGRES_DB="$ANUBIS_DB_NAME" \
    -p "${ANUBIS_DB_PORT}:5432" \
    --cpus 4 --memory 4096M \
    "$ANUBIS_DB_IMAGE" \
    -c shared_buffers=1GB \
    -c effective_cache_size=3GB \
    -c work_mem=32MB \
    -c maintenance_work_mem=256MB \
    -c random_page_cost=1.1 \
    -c track_io_timing=on \
    -c log_min_duration_statement=100 \
    -c max_parallel_workers_per_gather=4 >/dev/null
}

wait_ready() {
  for _ in $(seq 1 40); do
    if container exec "$ANUBIS_DB_CONTAINER" pg_isready -U "$ANUBIS_DB_USER" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "database did not become ready in 40s"
}

cmd_up() {
  container system status >/dev/null 2>&1 || { info "starting container runtime"; container system start; }
  if ! container_exists "$ANUBIS_DB_CONTAINER"; then
    create_container
  elif ! container_running "$ANUBIS_DB_CONTAINER"; then
    info "starting $ANUBIS_DB_CONTAINER"
    container start "$ANUBIS_DB_CONTAINER" >/dev/null
  fi
  wait_ready
  ok "database ready"
}

cmd_status() {
  if ! container_exists "$ANUBIS_DB_CONTAINER"; then
    warn "$ANUBIS_DB_CONTAINER does not exist — run: scripts/db.sh up"; return
  fi
  if container_running "$ANUBIS_DB_CONTAINER"; then
    ok "$ANUBIS_DB_CONTAINER running"
  else
    warn "$ANUBIS_DB_CONTAINER stopped"; return
  fi

  # Apple Container's default network is not routable from the host, so a
  # container created without --publish is reachable only via `container exec`.
  # That is fine for psql and the bench suite; the Go API will need the port.
  if port_busy "$ANUBIS_DB_PORT"; then
    ok "published on localhost:$ANUBIS_DB_PORT"
  else
    warn "not published on :$ANUBIS_DB_PORT — reachable only via 'container exec'"
    dim "  the Go API will need it. To republish (DESTROYS DATA, reseed with 'db.sh reset'):"
    dim "      scripts/db.sh recreate"
  fi

  psql_db -tAc "
    SELECT '  '||count(*)||' migrations applied' FROM schema_migrations
  " 2>/dev/null || dim "  schema not initialised — run: scripts/db.sh reset"
  psql_db -tAc "
    SELECT '  '||(SELECT count(*) FROM grants)||' grants, '||
           (SELECT count(*) FROM identities)||' identities, '||
           (SELECT count(*) FROM scope_nodes)||' scope nodes'
  " 2>/dev/null || true
}

# The migration files create schema_migrations but never wrote to it, so every
# environment reported "0 applied" regardless of state. Recording the checksum
# here means a file edited after being applied is detectable rather than silent.
# A database migrated before this runner existed has the full schema but an
# empty schema_migrations, so a naive run reapplies 0001 and fails on
# "relation already exists". Detect that and point at baseline rather than
# leaving the operator to decode the error.
cmd_migrate() {
  cmd_up
  local tracked tables
  tracked="$(psql_db -tAc "SELECT count(*) FROM schema_migrations" 2>/dev/null || echo 0)"
  tables="$(psql_db -tAc \
    "SELECT count(*) FROM pg_tables WHERE schemaname='public'" 2>/dev/null || echo 0)"
  if [ "$tracked" = "0" ] && [ "$tables" -gt 1 ]; then
    die "schema exists ($tables tables) but no migrations are recorded.
  This database predates migration tracking. Either:
      scripts/db.sh baseline   record the existing files as applied
      scripts/db.sh reset      DESTRUCTIVE: rebuild from scratch"
  fi
  local applied=0 skipped=0
  for f in "$ROOT"/migrations/*.sql; do
    local version sum seen
    version="$(basename "$f" .sql)"
    sum="$(shasum -a 256 "$f" | cut -d' ' -f1)"
    seen="$(psql_db -tAc \
      "SELECT checksum FROM schema_migrations WHERE version='$version'" 2>/dev/null || echo '')"
    if [ -n "$seen" ]; then
      [ "$seen" = "$sum" ] || warn "$version was modified after being applied (checksum drift)"
      skipped=$((skipped+1)); continue
    fi
    # --single-transaction matches the production runner
    # (internal/platform/migrate/runner.go wraps each file in one tx). Without
    # it a migration that fails halfway left dev databases in a state
    # production could never reach, and SET LOCAL — which migrations use to
    # bound DDL lock waits — silently did nothing.
    psql_db -v ON_ERROR_STOP=1 --single-transaction -q < "$f" >/dev/null || die "failed: $version"
    psql_db -q -c \
      "INSERT INTO schema_migrations (version, checksum) VALUES ('$version','$sum')
       ON CONFLICT (version) DO NOTHING" >/dev/null 2>&1 || true
    applied=$((applied+1))
  done
  ok "$applied applied, $skipped already present"
}

# Record every migration file as applied without running any of them. For
# adopting an existing database, never for a fresh one.
cmd_baseline() {
  cmd_up
  local n=0
  for f in "$ROOT"/migrations/*.sql; do
    local version sum
    version="$(basename "$f" .sql)"
    sum="$(shasum -a 256 "$f" | cut -d' ' -f1)"
    psql_db -q -c \
      "INSERT INTO schema_migrations (version, checksum) VALUES ('$version','$sum')
       ON CONFLICT (version) DO NOTHING" >/dev/null
    n=$((n+1))
  done
  ok "$n migrations recorded as applied (nothing was executed)"
}

cmd_reset() {
  cmd_up
  warn "dropping and rebuilding the schema"
  "$ROOT"/bench/rebuild.sh
}

cmd_recreate() {
  warn "this deletes the container and all data in it"
  printf '  type "yes" to continue: '; read -r reply
  [ "$reply" = "yes" ] || die "aborted"
  container rm -f "$ANUBIS_DB_CONTAINER" >/dev/null 2>&1 || true
  create_container
  wait_ready
  ok "container recreated and published on :$ANUBIS_DB_PORT"
  info "schema is empty — run: scripts/db.sh reset"
}

# Bring the database to a state the console can actually be used against:
# schema current, the development dataset loaded, and somebody who can sign in.
#
# Non-destructive on purpose. It is run on every `scripts/dev.sh`, and
# reseeding 150k grants because someone opened the console twice would be
# both slow and surprising.
cmd_seed() {
  cmd_up
  cmd_migrate
  local tenants
  tenants="$(psql_db -tAc "SELECT count(*) FROM tenants" 2>/dev/null || echo 0)"
  if [ "${tenants:-0}" -eq 0 ]; then
    info "loading the development dataset (bench/seed.sql)"
    psql_db -v ON_ERROR_STOP=1 -q < "$ROOT/bench/seed.sql" >/dev/null \
      || die "seed failed — run: scripts/db.sh reset"
    ok "dataset loaded"
  else
    dim "  $tenants tenants already present — not reseeding"
  fi
  cmd_devadmin
}

# bench/seed.sql creates tens of thousands of identities and NOT ONE password:
# it exists to benchmark authorize(), not to be signed into. So nothing in a
# freshly seeded database can use the console. This makes an account that can.
#
# The password is fixed and printed, because it is a development convenience
# and pretending otherwise would just mean everyone greps for it. It is
# refused outright when ANUBIS_ENV says this is production.
cmd_devadmin() {
  need go
  if [ "${ANUBIS_ENV:-dev}" = "prod" ]; then
    die "refusing to create a fixed-password admin with ANUBIS_ENV=prod"
  fi
  local pass="${ANUBIS_DEV_ADMIN_PASS:-anubis-dev-password}"
  local tenant="${ANUBIS_DEV_TENANT:-impack}"
  # A dedicated account, NOT 'admin'. bootstrap only creates an identity that
  # is absent, so reusing a human-managed admin would print a password that
  # does not open it — a dev script that lies is worse than no dev script.
  local user="${ANUBIS_DEV_ADMIN_USER:-devadmin}"

  # bootstrap looks each object up before creating it, so this is safe to run
  # on every start.
  # Two accounts, two populations: an ordinary PERSON inside $tenant for the
  # tenant-facing flows, and a PLATFORM owner who belongs to no tenant and is
  # the only kind of account that can administer anything.
  ( cd "$ROOT" && ANUBIS_DB_URL="$(db_url)" go run ./cmd/anubisd bootstrap \
      --tenant "$tenant" --name "Impack" \
      --admin-user "$user" --admin-pass "$pass" \
      --platform-user "$user" --platform-pass "$pass" >/dev/null 2>&1 ) \
    || die "bootstrap failed — run it directly to see why:
  ANUBIS_DB_URL='$(db_url)' go run ./cmd/anubisd bootstrap --tenant $tenant --admin-user $user --admin-pass '<pass>'"
  # This account belongs to the dev script, so the script guarantees it can be
  # signed into. A second factor enrolled during testing would otherwise lock
  # the environment behind an authenticator nobody still has — which is
  # exactly what happened once.
  local cleared
  cleared="$(psql_db -tAc "UPDATE platform_users
                              SET totp_secret_enc = NULL, totp_enrolled_at = NULL,
                                  totp_last_step = 0
                            WHERE lower(username) = lower('$user')
                              AND totp_enrolled_at IS NOT NULL
                        RETURNING username" 2>/dev/null || true)"
  if [ -n "$cleared" ]; then
    warn "cleared the second factor on '$user' so this environment stays signable-into"
    dim  "  to test 2FA, enrol from the console and keep the authenticator"
  fi

  ok "platform console: user '$user', password '$pass' (no tenant — operators belong to none)"
  dim "  tenant '$tenant' also holds a person '$user' for tenant-facing flows (no admin power)"
}

db_url() {
  printf 'postgres://%s:%s@localhost:%s/%s?sslmode=disable' \
    "$ANUBIS_DB_USER" "$ANUBIS_DB_USER" "$ANUBIS_DB_PORT" "$ANUBIS_DB_NAME"
}

case "${1:-status}" in
  up)       cmd_up ;;
  down)     container stop "$ANUBIS_DB_CONTAINER" >/dev/null && ok "stopped" ;;
  status)   cmd_status ;;
  migrate)  cmd_migrate ;;
  seed)     cmd_seed ;;
  devadmin) cmd_devadmin ;;
  baseline) cmd_baseline ;;
  reset)    cmd_reset ;;
  recreate) cmd_recreate ;;
  psql)     shift; cmd_up >/dev/null; container exec -it "$ANUBIS_DB_CONTAINER" \
              psql -U "$ANUBIS_DB_USER" -d "$ANUBIS_DB_NAME" "$@" ;;
  *)        die "unknown command '$1'
  usage: db.sh {up|down|status|migrate|seed|devadmin|baseline|reset|recreate|psql}" ;;
esac
