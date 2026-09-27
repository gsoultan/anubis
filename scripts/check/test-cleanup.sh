#!/usr/bin/env bash
# Test cleanups must be able to run, and must say when they fail.
#
# t.Cleanup runs after a test's defers, so `defer pool.Close()` shuts the pool
# before any cleanup can use it — and with the error discarded, nothing says
# so. Three tests did exactly that, and by 2026-09-26 the dev database held 79
# leftover operators among 81, ten probe tenants and ten test signing keys.
# Every one of those runs reported ok. See cmd/testcleanup for the two rules.
set -euo pipefail
cd "$(dirname "$0")/../.."

if ! out="$(go run ./cmd/testcleanup 2>&1)"; then
  echo "FAIL: test cleanups that cannot run, or cannot report failing:" >&2
  printf '%s\n' "$out" | sed 's/^/  /' >&2
  exit 1
fi

echo "ok: every test cleanup can run and reports its failures"
