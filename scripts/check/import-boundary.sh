#!/usr/bin/env bash
# internal/domain imports NOTHING outside the Go standard library
# (ADR-0002 rule 2). Enforced, not reviewed.
set -euo pipefail
cd "$(dirname "$0")/../.."
bad=$(go list -deps ./internal/domain/ | grep -v '^github.com/gsoultan/anubis/internal/domain$' | grep '\.' | grep -v '^golang.org/x/' || true)
# stdlib packages have no dot in the first path element; anything with a dot
# (a domain) is third-party. x/ is also banned for domain.
bad=$(go list -deps ./internal/domain/ | awk -F/ '$1 ~ /\./ {print}' | grep -v '^github.com/gsoultan/anubis/internal/domain$' || true)
if [ -n "$bad" ]; then
  echo "FAIL: internal/domain imports non-stdlib packages:" >&2
  echo "$bad" >&2
  exit 1
fi
echo "ok: internal/domain is stdlib-only"
