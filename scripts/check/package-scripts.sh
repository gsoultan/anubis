#!/usr/bin/env bash
# The package maintainer scripts decide whether anubisd is still running, and
# still enabled at boot, after somebody upgrades the package. Nothing else
# exercises them before a real host does.
#
# dpkg and rpm both run the OLD package's pre-remove script during an
# upgrade, not only on removal:
#   dpkg: prerm upgrade <new-version>        (removal: prerm remove)
#   rpm:  %preun with $1 = 1                 (erase:   %preun with $1 = 0)
# A pre-remove that disables the unit on every call turns each upgrade into a
# host that comes back from its next reboot with no identity service.
#
# systemctl is stubbed so the calls can be recorded; the script's check for a
# running systemd is pointed at a directory this test controls.
set -euo pipefail
cd "$(dirname "$0")/../.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/bin" "$work/run-systemd"
cat > "$work/bin/systemctl" <<'STUB'
#!/bin/sh
echo "$*" >> "$SYSTEMCTL_LOG"
STUB
chmod +x "$work/bin/systemctl"

sed "s#/run/systemd/system#$work/run-systemd#g" packaging/preremove.sh > "$work/preremove.sh"

fail=0
# expect <want: stop|keep> <args...>
expect() {
  want="$1"; shift
  log="$work/calls"; : > "$log"
  SYSTEMCTL_LOG="$log" PATH="$work/bin:$PATH" sh "$work/preremove.sh" "$@"
  if grep -q disable "$log"; then got=stop; else got=keep; fi
  if [ "$got" != "$want" ]; then
    echo "FAIL: preremove $* — want it to $want the service, it would $got it (systemctl: $(tr '\n' ';' < "$log"))" >&2
    fail=1
  fi
}

expect stop remove           # dpkg: package removed
expect stop 0                # rpm:  package erased
expect keep upgrade 0.7.0    # dpkg: old package's prerm during an upgrade
expect keep failed-upgrade 0.7.0
expect keep 1                # rpm:  old package's %preun during an upgrade

[ "$fail" -eq 0 ] || exit 1
echo "ok: package scripts stop anubisd on removal and leave it running on upgrade"
