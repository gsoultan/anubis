#!/usr/bin/env bash
# Build the admin console into ui/dist, then PROVE it is there.
#
# The binary embeds ui/dist (ui/embed.go). The copy committed to git is a
# placeholder that says "console not built" — so a release whose console
# build silently failed still compiles, still starts, and serves that page to
# operators instead of the console. The grep is the whole point of this
# script: it turns a silent wrong artefact into a failed build.
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v bun >/dev/null 2>&1; then
  echo "FATAL: bun is required to build the console" >&2
  exit 1
fi

# The console is compiled INTO the release binary, so the bundler version is
# part of what ships. CI and the Dockerfile pin 1.4.0; warn when a local
# build would produce something they cannot reproduce.
want="${ANUBIS_BUN_VERSION:-1.4.0}"
have="$(bun --version 2>/dev/null || echo unknown)"
if [ "$have" != "$want" ]; then
  echo "WARNING: bun $have, but CI and the Dockerfile build with $want." >&2
  echo "         The bundle may differ from what ships. Install $want to match." >&2
fi

(cd ui && bun install --frozen-lockfile && bun run build)

if grep -q "Console not built" ui/dist/index.html; then
  echo "FATAL: ui/dist/index.html is still the placeholder — the console did not build" >&2
  exit 1
fi
echo "console built: $(wc -c < ui/dist/index.html) bytes of shell, $(ls ui/dist | wc -l) artefacts"
