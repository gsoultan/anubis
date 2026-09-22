#!/usr/bin/env bash
# The committed ui/dist/index.html must stay the placeholder.
#
# ui/dist/ is gitignored except for index.html, which is tracked so
# `go:embed all:dist` has something to embed in a fresh checkout. Building the
# console locally overwrites it, so it is one `git add -A` away from being
# committed — and it was, at some point before this check existed.
#
# What that costs: the committed shell then names chunk files that are NOT in
# the repository. Requests for them fall through to the SPA catch-all, arrive
# as text/html under X-Content-Type-Options: nosniff, and the browser refuses
# to execute them. An operator gets a blank page and two console errors rather
# than a sentence saying the console was not built. Verified by serving a
# binary built from exactly that state.
#
# It also disarms the guard in scripts/build-console.sh, which proves the
# build happened by grepping the output for "Console not built". Once the
# committed copy no longer contains that string, a build that produced nothing
# passes too.
#
# Checked against the COMMITTED blob rather than the working tree: a developer
# with a locally built console is fine and expected, committing it is not.
set -euo pipefail
cd "$(dirname "$0")/../.."

blob="$(git show HEAD:ui/dist/index.html 2>/dev/null || true)"
if [ -z "$blob" ]; then
  echo "FAIL: ui/dist/index.html is not committed — go:embed all:dist needs it" >&2
  exit 1
fi

if ! printf '%s' "$blob" | grep -q "Console not built"; then
  echo "FAIL: the committed ui/dist/index.html is not the placeholder." >&2
  echo "" >&2
  echo "It looks like a built console shell was committed. The chunk files it" >&2
  echo "names are gitignored, so a fresh checkout serves a blank page." >&2
  echo "" >&2
  echo "  git checkout \$(git log -1 --format=%H -- ui/dist/index.html)~1 -- ui/dist/index.html" >&2
  echo "" >&2
  echo "or restore the placeholder by hand, then rebuild with" >&2
  echo "scripts/build-console.sh when you need the real console." >&2
  exit 1
fi

# A placeholder that fetches anything can fail the same way it is warning
# about. It has to stand on its own.
if printf '%s' "$blob" | grep -qE '<(script|link)[^>]+(src|href)='; then
  echo "FAIL: the committed ui/dist/index.html references external assets." >&2
  echo "The page that explains a missing console must not depend on files" >&2
  echo "that are also missing — keep its styles inline and its scripts absent." >&2
  exit 1
fi

echo "ok: the committed console shell is the placeholder"
