#!/bin/sh
set -e

# A dedicated, unprivileged, non-login account. The service writes nothing to
# disk, so it needs no home directory worth the name.
if ! getent group anubis >/dev/null 2>&1; then
    groupadd --system anubis
fi
if ! getent passwd anubis >/dev/null 2>&1; then
    useradd --system --gid anubis --no-create-home \
            --home-dir /nonexistent --shell /usr/sbin/nologin \
            --comment "Anubis identity service" anubis
fi

chown root:anubis /etc/anubis
chmod 0750 /etc/anubis
[ -f /etc/anubis/anubisd.env ] && chown root:anubis /etc/anubis/anubisd.env && chmod 0640 /etc/anubis/anubisd.env

# The secrets directory is root-only: systemd reads the credential as root
# and mounts it into the unit, so the service account never needs access.
chown root:root /etc/anubis/secrets
chmod 0700 /etc/anubis/secrets

systemctl daemon-reload >/dev/null 2>&1 || true

# An UPGRADE must not read like a fresh install. Printing "create the master
# key" to somebody who already has one is, at best, noise they learn to skip
# — and at worst an instruction that destroys every sealed key in their
# database if followed. The key file is the reliable marker: it exists on
# every configured install and on no fresh one.
if [ -f /etc/anubis/secrets/master.key ]; then
    systemctl is-active --quiet anubisd 2>/dev/null && RUNNING=yes || RUNNING=no
    cat <<UPGRADE

anubisd upgraded. Your master key and configuration were left untouched.

  1. Apply any new migrations, as the OWNER role. Forward-only, and they
     take an advisory lock so several hosts cannot race:

       ANUBIS_DB_URL=postgres://anubis_owner:...@host/anubis anubisd migrate

  2. Restart:

       systemctl restart anubisd

  3. Confirm it came back:

       curl -sf http://127.0.0.1:7448/readyz && echo ready

UPGRADE
    [ "$RUNNING" = yes ] && echo "  (anubisd is running the OLD binary until you restart it.)" && echo
    exit 0
fi

cat <<'BANNER'

anubisd installed. Before starting it:

  1. Create the master key (32 bytes, base64url). Losing it makes every
     signing key and every PII key unreadable — back it up somewhere that
     is not this host:

       umask 077
       head -c 32 /dev/urandom | basenc --base64url | tr -d '=' \
         > /etc/anubis/secrets/master.key
       chmod 0400 /etc/anubis/secrets/master.key

  2. Set ANUBIS_DB_URL and ANUBIS_ISSUER in /etc/anubis/anubisd.env. Behind
     a TLS proxy, set ANUBIS_TRUSTED_PROXIES too, or every caller shares one
     rate-limit bucket.

  3. Apply the schema as the OWNER role (a separate step on purpose, so a
     schema change is a deliberate deploy with its own rollback plan):

       ANUBIS_DB_URL=postgres://anubis_owner:...@host/anubis anubisd migrate

  4. Mint the first signing key. A production install deliberately does NOT
     generate one on its own, so until this runs the service answers /readyz
     with 503 and cannot issue a single token:

       anubisd keys init access

  5. systemctl enable --now anubisd

Then open the console on the API port. First run shows the installer.
Runbook: /usr/share/doc/anubisd or docs/operations.md.

BANNER
