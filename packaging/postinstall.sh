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

cat <<'BANNER'

anubisd installed. Before starting it:

  1. Create the master key (32 bytes, base64url). Losing it makes every
     signing key and every PII key unreadable — back it up somewhere that
     is not this host:

       umask 077
       head -c 32 /dev/urandom | basenc --base64url | tr -d '=' \
         > /etc/anubis/secrets/master.key
       chmod 0400 /etc/anubis/secrets/master.key

  2. Set ANUBIS_DB_URL and ANUBIS_ISSUER in /etc/anubis/anubisd.env.

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
