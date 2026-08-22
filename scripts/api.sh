#!/usr/bin/env bash
# API dev server. Placeholder until the Go service exists (see docs/roadmap.md).
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

if [ ! -f "$ROOT/go.mod" ]; then
  warn "no Go module yet — the API is not built (docs/roadmap.md, phase 0-1)"
  dim  "  the console runs against its in-memory backend until then"
  exit 0
fi

need go
require_port "$ANUBIS_API_PORT" "api" "API"
"$ROOT/scripts/db.sh" up

info "api   http://localhost:$ANUBIS_API_PORT"
cd "$ROOT"
exec env \
  ANUBIS_DB_URL="postgres://${ANUBIS_DB_USER}:${ANUBIS_DB_USER}@localhost:${ANUBIS_DB_PORT}/${ANUBIS_DB_NAME}?sslmode=disable" \
  ANUBIS_LISTEN=":${ANUBIS_API_PORT}" \
  go run ./cmd/anubisd
