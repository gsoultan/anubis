#!/usr/bin/env bash
# Build every buildable workspace.
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
need bun

info "console"
cd "$ROOT/ui"
[ -d node_modules ] || bun install
bun run build
ok "console → ui/dist"

if [ -f "$ROOT/go.mod" ]; then
  info "api"
  cd "$ROOT"; go build -o bin/anubisd ./cmd/anubisd
  ok "api → bin/anubisd"
else
  dim "api: no Go module yet (docs/roadmap.md)"
fi
