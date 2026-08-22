#!/usr/bin/env bash
# Domain layers import nothing outside the standard library (ADR-0002 rule 2,
# now enforced per bounded context). A domain package that reaches for pgx,
# connect or go-kit has stopped being a domain package.
set -euo pipefail
cd "$(dirname "$0")/../.."
fail=0
for pkg in $(go list ./internal/*/domain/... ./internal/shared/... 2>/dev/null); do
  bad=$(go list -deps "$pkg" | awk -F/ '$1 ~ /\./ {print}' \
        | grep -v '^github.com/gsoultan/anubis/internal/' || true)
  if [ -n "$bad" ]; then
    echo "FAIL: $pkg imports non-stdlib packages:" >&2
    echo "$bad" >&2
    fail=1
  fi
done
[ "$fail" = "0" ] || exit 1
echo "ok: domain and shared packages are stdlib-only"
