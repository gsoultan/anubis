#!/usr/bin/env bash
# Look inside the built .deb and .rpm files for what an installed host needs.
#
# Runs after a goreleaser snapshot (ci.yml, release-build). A missing entry in
# .goreleaser.yaml's nfpms.contents still builds a package — it just installs
# a host without the file. The install message in packaging/postinstall.sh
# sends operators to /usr/share/doc/anubisd for the runbook, so the runbook
# has to be there.
#
# Usage: scripts/ci/package-contents.sh [dist-dir]
set -euo pipefail
dist="${1:-dist}"

want=(
  /usr/bin/anubisd
  /lib/systemd/system/anubisd.service
  /etc/anubis/anubisd.env
  /etc/anubis/secrets
  /usr/share/doc/anubisd/README.md
  /usr/share/doc/anubisd/LICENSE
  /usr/share/doc/anubisd/operations.md
  /usr/share/doc/anubisd/go-live.md
  /usr/share/doc/anubisd/anubis.rules.yml
)

list() {
  case "$1" in
    *.deb) dpkg-deb -c "$1" | awk '{print $6}' | sed -e 's#^\./#/#' -e 's#/$##' ;;
    *.rpm) rpm -qlp "$1" 2>/dev/null ;;
  esac
}

shopt -s nullglob
pkgs=("$dist"/*.deb "$dist"/*.rpm)
[ "${#pkgs[@]}" -gt 0 ] || { echo "FAIL: no .deb or .rpm in $dist — run goreleaser first" >&2; exit 1; }

fail=0
for p in "${pkgs[@]}"; do
  files="$(list "$p")"
  [ -n "$files" ] || { echo "FAIL: could not list $p" >&2; fail=1; continue; }
  for f in "${want[@]}"; do
    if ! grep -qxF "$f" <<<"$files"; then
      echo "FAIL: $(basename "$p") does not install $f" >&2
      fail=1
    fi
  done
done

[ "$fail" -eq 0 ] || exit 1
echo "ok: ${#pkgs[@]} packages install the binary, unit, config and runbook"
