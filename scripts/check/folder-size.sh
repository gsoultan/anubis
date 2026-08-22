#!/usr/bin/env bash
# House rule: no folder holds more than 10 Go files. It forces the domain
# carve to stay honest — when a package outgrows the limit, the answer is a
# missing concept, not a bigger folder. Generated code is exempt (sqlc and
# buf decide their own file counts; each context still gets its own package).
set -euo pipefail
cd "$(dirname "$0")/../.."
LIMIT=10
fail=0
while IFS= read -r d; do
  case "$d" in */gen|*/gen/*|./gen/*) continue ;; esac
  n=$(find "$d" -maxdepth 1 -name '*.go' | wc -l | tr -d ' ')
  if [ "$n" -gt "$LIMIT" ]; then
    echo "FAIL: $d holds $n Go files (limit $LIMIT)" >&2
    fail=1
  fi
done < <(find ./internal ./pkg ./cmd ./test -type d 2>/dev/null)
[ "$fail" = "0" ] || exit 1
echo "ok: no folder exceeds $LIMIT Go files"
