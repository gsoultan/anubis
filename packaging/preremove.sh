#!/bin/sh
set -e

# dpkg and rpm run this script during an UPGRADE as well as on removal. Only a
# removal may stop and disable the service: an upgrade that disables it leaves
# a host whose next reboot comes back without anubisd. On upgrade the old
# binary keeps running until the operator restarts it after `anubisd migrate`
# (postinstall.sh says so).
#   dpkg prerm:  remove | upgrade | deconfigure | failed-upgrade
#   rpm %preun:  0 (erase) | 1 (upgrade)
case "${1:-remove}" in
    remove|0) ;;
    *) exit 0 ;;
esac

if [ -d /run/systemd/system ]; then
    systemctl --no-reload disable --now anubisd.service >/dev/null 2>&1 || true
fi
# The account and /etc/anubis/secrets are deliberately LEFT BEHIND. Removing
# a package must not destroy the master key: without it every signing key and
# every encrypted column in the database is unreadable, including after a
# reinstall.
