#!/usr/bin/env bash
# Regenerate all generated code: proto -> Go (gen/go), proto -> TS (ui/src/gen),
# sql -> Go (internal/adapter/postgres/gen). Generated output is committed;
# CI re-runs this and fails on drift.
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

export PATH="$(go env GOPATH)/bin:$PATH"

need() { command -v "$1" >/dev/null 2>&1 || die "$1 not installed ($2)"; }
need buf  "https://buf.build — or: go install github.com/bufbuild/buf/cmd/buf@latest"
need sqlc "go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"

cd "$ROOT"

info "buf lint"
buf lint

info "proto -> go"
buf generate

if [ -d "$ROOT/ui/node_modules/@bufbuild/protoc-gen-es" ]; then
  info "proto -> ts"
  (cd "$ROOT/ui" && buf generate --template buf.gen.yaml "$ROOT/proto")
else
  dim "  ui/node_modules missing @bufbuild/protoc-gen-es — skipping TS gen (run: cd ui && bun install)"
fi

info "sqlc"
sqlc generate

ok "generation complete"
