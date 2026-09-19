#!/usr/bin/env bash
# API dev server.
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

need go
require_port "$ANUBIS_API_PORT" "api" "API"
"$ROOT/scripts/db.sh" up

# Build then exec, rather than `go run`. `go run` compiles to its cache and runs
# the result as a *child*, so the PID dev.sh records is `go run` while the
# process actually holding the port is one level down and never sees the kill.
# Ctrl-C hides this — the terminal signals the whole foreground group — but a
# closed terminal leaves a live anubisd on the API port that blocks the next
# start. Exec'ing the binary makes the PID dev.sh tracks the one serving.
cd "$ROOT"
info "building api"
go build -o bin/anubisd ./cmd/anubisd

info "api   http://localhost:$ANUBIS_API_PORT"
exec env \
  ANUBIS_DB_URL="postgres://${ANUBIS_DB_USER}:${ANUBIS_DB_USER}@localhost:${ANUBIS_DB_PORT}/${ANUBIS_DB_NAME}?sslmode=disable" \
  ANUBIS_LISTEN=":${ANUBIS_API_PORT}" \
  "$ROOT/bin/anubisd" serve
