#!/usr/bin/env bash
# Generated code is committed; regeneration must be a no-op. Drift means a
# .proto or .sql changed without regenerating (or vice versa).
set -euo pipefail
cd "$(dirname "$0")/../.."
export PATH="$(go env GOPATH)/bin:$PATH"
buf generate
sqlc generate
if ! git diff --exit-code --quiet gen internal/adapter/postgres/gen; then
  echo "FAIL: generated code drifted — run scripts/gen.sh and commit" >&2
  git --no-pager diff --stat gen internal/adapter/postgres/gen >&2
  exit 1
fi
echo "ok: generated code matches sources"
