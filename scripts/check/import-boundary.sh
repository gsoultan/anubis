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

# The unguarded entry points. Two usecases take their own tenant id and check
# no operator, because the caller is a scheduler and there is no operator to
# check. That is safe exactly as long as nothing reachable from the network
# can call them: a transport supplying its own tenant id has proved nothing.
for sym in ApplyDocumentAsSystem RunDue; do
  hits=$(grep -rn "\.$sym(" internal/*/adapter/rpc internal/*/adapter/http internal/api 2>/dev/null || true)
  if [ -n "$hits" ]; then
    echo "FAIL: $sym is unguarded and must not be called from a transport:" >&2
    echo "$hits" >&2
    fail=1
  fi
done

[ "$fail" = "0" ] || exit 1
echo "ok: domain and shared packages are stdlib-only; unguarded applies are not reachable from a transport"
