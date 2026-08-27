#!/usr/bin/env bash
# ADR-0002 is a promise about the dependency list: no third-party libraries
# except the infrastructure Anubis cannot reasonably write itself, and no
# third-party cryptography at all. Nothing enforced it, so the promise was
# only as good as whoever last read the ADR.
#
# It also checks go.mod is tidy. That is not housekeeping: an untidy file
# misreports what the project actually depends on. The MySQL driver sat marked
# `// indirect` while being a direct import of cmd/anubisd/syncengines, which
# is how a dependency ends up in the tree without anyone weighing it.
set -euo pipefail
cd "$(dirname "$0")/../.."

# Every third-party module the tree may require directly, and why it earns it.
#   connectrpc.com/connect      transport (RPC over HTTP, gRPC-compatible)
#   github.com/go-kit/kit       endpoint/middleware plumbing
#   github.com/gsoultan/storm   query codegen used by the authz hot path
#   github.com/jackc/pgx/v5     Postgres driver — the storage engine
#   github.com/go-sql-driver/mysql   a sync SOURCE driver (ADR: any engine)
#   google.golang.org/protobuf  generated-code runtime
ALLOWED='connectrpc.com/connect
github.com/go-kit/kit
github.com/gsoultan/storm
github.com/jackc/pgx/v5
github.com/go-sql-driver/mysql
google.golang.org/protobuf'

fail=0

for mod in . pkg/anubis; do
  # Direct requirements only: indirect ones are the allowed modules' own
  # business, and judging those is what `go mod tidy` and govulncheck are for.
  direct="$(cd "$mod" && awk '/^require \(/,/^\)/' go.mod \
    | grep -v '// indirect' \
    | awk '{print $1}' \
    | grep -E '^[a-z]' \
    | grep -vx 'require' \
    | grep -v '^github.com/gsoultan/anubis' || true)"

  while IFS= read -r dep; do
    [ -z "$dep" ] && continue
    if ! printf '%s\n' "$ALLOWED" | grep -qxF "$dep"; then
      echo "FAIL: $mod/go.mod requires $dep, which ADR-0002 does not allow." >&2
      echo "      Add it to scripts/check/deps.sh only with a reason in the ADR." >&2
      fail=1
    fi
  done <<< "$direct"
done

# Crypto is stdlib, without exception. golang.org/x/crypto is the one people
# reach for by reflex, so name it rather than waiting for the allowlist to
# catch it after somebody has already built on it.
# Anubis's own paseto and kdf packages are the stdlib implementations this
# rule exists to require, so they are not what it is looking for.
crypto_hits="$(grep -rn --include='*.go' -E '"(golang\.org/x/crypto|github\.com/[^"]*/(bcrypt|scrypt|argon2|jwt|jose|paseto))' \
  cmd internal pkg 2>/dev/null \
  | grep -v '"github.com/gsoultan/anubis/' || true)"
if [ -n "$crypto_hits" ]; then
  echo "FAIL: third-party cryptography imported (ADR-0002: stdlib only):" >&2
  echo "$crypto_hits" >&2
  fail=1
fi

# An untidy go.mod misreports the dependency list this script just checked.
for mod in . pkg/anubis; do
  if ! (cd "$mod" && go mod tidy -diff >/dev/null 2>&1); then
    echo "FAIL: $mod/go.mod is not tidy — run 'go mod tidy' and commit." >&2
    (cd "$mod" && go mod tidy -diff 2>&1 | head -20) >&2
    fail=1
  fi
done

[ "$fail" -eq 0 ] || exit 1
echo "ok: dependencies are on the ADR-0002 allowlist, crypto is stdlib, go.mod is tidy"
