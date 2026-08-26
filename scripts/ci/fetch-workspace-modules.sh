#!/usr/bin/env bash
# go.work lists sibling modules by relative path (`use ../raorm`). Those paths
# exist on a developer's machine and nowhere else, so CI — and any fresh
# clone — must materialise them or every `go` command fails with:
#
#   cannot load module ../raorm listed in go.work file
#
# Each entry is cloned at ANUBIS_RAORM_REF (default main).
set -euo pipefail
cd "$(dirname "$0")/../.."

ref="${ANUBIS_RAORM_REF:-main}"
root="$(cd .. && pwd)"

# Only the siblings — '.' and './pkg/anubis' are in this repository already.
grep -oE '^\s*\.\./[A-Za-z0-9._-]+' go.work 2>/dev/null | tr -d ' ' | while read -r rel; do
  name="${rel#../}"
  dest="$root/$name"
  if [ -d "$dest/.git" ] || [ -f "$dest/go.mod" ]; then
    echo "workspace module $name: already present"
    continue
  fi
  echo "workspace module $name: cloning gsoultan/$name@$ref"
  if ! git clone --depth 1 --branch "$ref" \
        "https://github.com/gsoultan/$name.git" "$dest" 2>&1 | tail -2; then
    cat >&2 <<MSG

FATAL: go.work lists '$rel' but gsoultan/$name has no branch '$ref'.

The module exists only on the machine that wrote go.work. Until it is
pushed, this repository cannot be built anywhere else — not in CI, and not
by anyone who clones it. Either publish it, or take the sibling out of
go.work and depend on a released version.
MSG
    exit 1
  fi
done
