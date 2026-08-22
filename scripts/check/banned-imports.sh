#!/usr/bin/env bash
# math/rand is banned repository-wide (docs/security.md rule 1): an auth
# system that draws from a PRNG has shipped this bug before.
set -euo pipefail
cd "$(dirname "$0")/../.."
hits=$(grep -rn --include='*.go' -E '"math/rand(/v2)?"' cmd internal pkg 2>/dev/null \
  | grep -v 'internal/adapter/postgres/gen/' || true)
if [ -n "$hits" ]; then
  echo "FAIL: math/rand imported:" >&2
  echo "$hits" >&2
  exit 1
fi
echo "ok: no math/rand"
