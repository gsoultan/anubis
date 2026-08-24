#!/usr/bin/env bash
# Bounded contexts talk through ports and domain types, never by reaching
# into another context's adapters. An import of <other>/adapter/... is a
# layering violation: it couples this context to how that one stores or
# serves its data.
set -euo pipefail
cd "$(dirname "$0")/../.."
fail=0
for ctx in identity auth authz scope tenancy audit gate provisioning; do
  for other in identity auth authz scope tenancy audit gate provisioning; do
    [ "$ctx" = "$other" ] && continue
    hits=$(grep -rn --include='*.go' \
      "gsoultan/anubis/internal/$other/adapter" "internal/$ctx" 2>/dev/null || true)
    if [ -n "$hits" ]; then
      # the auth transport's token mapper is shared with authz on purpose
      hits=$(echo "$hits" | grep -v 'auth/adapter/rpc"' || true)
    fi
    if [ -n "$hits" ]; then
      echo "FAIL: $ctx imports $other's adapters:" >&2
      echo "$hits" >&2
      fail=1
    fi
  done
done
[ "$fail" = "0" ] || exit 1
echo "ok: no context reaches into another context's adapters"
