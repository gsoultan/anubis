#!/bin/sh
set -e
if [ -d /run/systemd/system ]; then
    systemctl --no-reload disable --now anubisd.service >/dev/null 2>&1 || true
fi
# The account and /etc/anubis/secrets are deliberately LEFT BEHIND. Removing
# a package must not destroy the master key: without it every signing key and
# every encrypted column in the database is unreadable, including after a
# reinstall.
