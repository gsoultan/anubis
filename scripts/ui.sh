#!/usr/bin/env bash
# Console dev server.
#
#   scripts/ui.sh                     port 7447
#   ANUBIS_UI_PORT=9999 scripts/ui.sh
#
# Dev runs through Vite even though `bun run build` uses Bun's native bundler:
# Vite keeps React Fast Refresh and the TanStack Router plugin, which
# regenerates the route tree when a file appears under src/routes. Matching that
# under Bun needs a second `tsr watch` process — one moving part too many in dev.
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
need bun

require_port "$ANUBIS_UI_PORT" "console" "UI"
cd "$ROOT/ui"
[ -d node_modules ] || { info "installing dependencies"; bun install; }

info "console   http://localhost:$ANUBIS_UI_PORT"
dim  "  /v1 → http://localhost:$ANUBIS_API_PORT (API not built yet; the console uses its in-memory backend)"
echo
exec bun run --bun vite --port "$ANUBIS_UI_PORT" --strictPort
