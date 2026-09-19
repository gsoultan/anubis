#!/usr/bin/env bash
# Formatting is not a matter of taste here: gofmt is the one canonical answer,
# so an unformatted file is noise in every future diff that touches it. Three
# files had drifted before this gate existed, which is exactly how it happens
# — nothing was checking, so nothing said.
set -euo pipefail
cd "$(dirname "$0")/../.."
unformatted=$(gofmt -l cmd internal pkg test 2>/dev/null || true)
if [ -n "$unformatted" ]; then
  echo "FAIL: not gofmt'd — run 'gofmt -w' on:" >&2
  echo "$unformatted" >&2
  exit 1
fi
echo "ok: every Go file is gofmt'd"
