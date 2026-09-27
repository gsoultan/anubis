# Go-live checklist

A sequenced first-install runbook. [operations.md](operations.md) is the
reference — every step here links the section that explains it; this page is
the order to do them in, and the gates that must be green before real users
arrive. Written against **v0.4.4**.

This is a checklist, not a promise the software is ready for you. Three things
no test in this repository can supply are still open, and they are the point of
a staged rollout: a real deployment (everything here has only been drilled on a
laptop against containers), an outside security review, and a week of real
traffic. See [roadmap.md](roadmap.md#what-is-left-is-not-code).

## Before the first boot

- [ ] **Get the artefact you can verify.** Download the release from the
  [tags page](https://github.com/gsoultan/anubis/releases), then check the
  signature before trusting the binary:
  ```
  cosign verify-blob \
    --certificate checksums.txt.pem --signature checksums.txt.sig \
    --certificate-identity-regexp '^https://github.com/gsoultan/anubis/\.github/workflows/release\.yml@refs/tags/vX\.Y\.Z$' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    checksums.txt
  ```
  Then `sha256sum -c checksums.txt`. A `.deb`/`.rpm` carries an SBOM alongside.
- [ ] **Provision Postgres and the database roles.** The runtime connects as
  `anubis_app`, migrations run as `anubis_owner`, reporting reads as
  `anubis_readonly` — three roles, least privilege, enforced by
  `migrations/0037`. See [Database roles](operations.md#database-roles). The
  role separation is a real control (`TestDatabaseRolesCannotExceedTheirPrivilege`),
  not a formality: do not run everything as the owner.
- [ ] **Hold the master key in a KMS, not an environment variable.** Prefer
  `ANUBIS_KEY_FILE` (the systemd unit mounts it with `LoadCredential`) over
  `ANUBIS_MASTER_KEY` — an env var is readable from `/proc` and survives core
  dumps. base64url, 32 bytes. Losing it means every signing key and every
  sealed PII key is unrecoverable; escrow it. See
  [Configuration](operations.md#configuration).
- [ ] **Set the production-shaped configuration.** `ANUBIS_ENV=prod` (refuses
  to boot without a master key), `ANUBIS_ISSUER` = the exact public URL your
  SDKs verify, `ANUBIS_TRUSTED_PROXIES` = your TLS proxy's CIDRs (without it
  every caller shares one rate-limit bucket), and `ANUBIS_DEFAULT_TENANT` for a
  single-tenant install (the default is a dev fixture name). `ANUBIS_AUTOKEYS`
  stays **off** in prod — a signing key minted automatically is one nobody
  escrowed; provision keys deliberately.

## Bring it up

- [ ] **Migrate, then bootstrap.** `anubisd migrate` (as the owner) is a
  separate step on purpose. Then `anubisd bootstrap` creates the first tenant
  and the first platform owner — without it the console shows an empty state
  that reads as "nothing here" but is really "you cannot get in". See
  [Deploying](operations.md#deploying).
- [ ] **Terminate TLS at a reverse proxy** (nginx or Caddy configs are given)
  and confirm the console is served **same-origin** — `ANUBIS_UI_ORIGIN` is a
  dev-only CORS escape hatch and must be unset in prod. See
  [TLS and the reverse proxy](operations.md#tls-and-the-reverse-proxy).
- [ ] **Confirm readiness gates on snapshot age.** Past `ANUBIS_SNAPSHOT_MAX_AGE`
  the gate fails closed *and* readiness fails, so a stale instance pulls itself
  from the balancer. Verify your load balancer honours `/readyz`. See
  [Health and readiness](operations.md#health-and-readiness).

## Prove it before users arrive

- [ ] **Scrape metrics and load the alert rules.** `/metrics` is Prometheus;
  the rules ship in `packaging/`. `anubis_audit_dropped_total` and
  `token.reuse_detected` are the two that must page a human. See
  [Metrics](operations.md#metrics) and [alerting.md](alerting.md).
- [ ] **Perform the restore drill — this is a gate, not a nicety.** Take a
  backup, restore it to a scratch database, and confirm a refresh token from
  before the backup stays dead (its session is revoked and `token_epoch` has
  advanced). A backup you have never restored is a hope, not a control. See
  [Restoring from backup](operations.md#restoring-from-backup), which walks the
  drill.
- [ ] **Rehearse two incidents** so the runbook is not first read under load:
  [nobody can sign in to the console](operations.md#incident-nobody-can-sign-in-to-the-console)
  and [refresh token reuse](operations.md#incident-refresh-token-reuse).
- [ ] **Verify the audit chain end to end.** From the console's Audit screen,
  run **Verify chain** against real history; it recomputes every hash and names
  the first entry that breaks. The signed hourly anchors (`0049`) mean even a
  wholesale rewrite is caught. See
  [Verifying the audit log](security.md#verifying-the-audit-log).

## Integrator-facing notes for this version

Tell the teams whose applications call Anubis:

- **Refreshing:** a standard OIDC client refreshes at `POST /v1/token` with
  `grant_type=refresh_token` (v0.4.2). A refreshed token keeps its
  application's `aud`, format and lifetimes.
- **Enrolling an authenticator or managing sessions** for a person needs a
  token Anubis issued for itself (`AuthService.Login`), taken just before the
  call — not the token an application received from the OIDC flow (v0.4.2,
  v0.4.3). See the confused-deputy control in
  [security.md](security.md#confused-deputy--cross-service-token-replay).
- **Verifying tokens:** use `pkg/anubis`. It checks `iss`, `aud`, expiry and
  the signature, and refuses the algorithm-confusion shapes JOSE is known for.
  Do not hand-roll a verifier.

## What this checklist does not cover

- The **outside security review**. The crypto is stdlib, the properties are
  tested — by the people who wrote them. Someone who did not write it should
  read the token paths, the guards and the gate. `security.md` is the map.
- A **pilot**. Start with one tenant and one application for a couple of weeks
  before widening. The load figures are synthetic; the first real directory
  signing in is the test that matters.
