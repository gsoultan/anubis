# Changelog

Each release's annotated tag carries the full account — `git show v0.4.0` —
and this file is the index into them: what changed, what breaks, and what to
do about it before upgrading.

Pre-1.0, a minor bump carries deliberate behaviour changes and a patch does
not. Releases are built and signed by tag and published by hand, so a tag
existing does not mean a release was ever meant to be installed.

## v0.4.0 — 2026-09-23

**Action required: `POST /v1/login` now needs a CSRF token.** The hosted
sign-in form carries one and the server sets a matching cookie; a submission
without both is refused. Anything posting straight at that endpoint — a
script, a test harness, an integration — stops working until it fetches the
page first and submits what that page contains.

Logout has had this since it was written. Login did not, and there was no
Origin or Referer check either, so any site could auto-submit a form with its
own credentials and leave the visitor holding an SSO cookie for somebody
else's account. `SameSite` does not prevent it: it decides whether a cookie is
sent, not whether one can be set.

Behaviour you will notice:

- **Disabling an operator takes effect immediately.** It waited for their
  access token to expire — up to an hour of unchanged authority on the most
  privileged accounts in an installation. The token epoch was written on
  every platform token and never read back.
- **A factor-enrolment grace period now says so.** Signing in during one
  shows the deadline and offers to set the factor up, rather than redirecting
  straight through. Browser-only members previously met the policy for the
  first time on the day it refused them.
- **The hosted second-factor step works.** It never had: the form carries no
  password field and the handler demanded one, so a correct code was answered
  with "Invalid username or password".
- A realm's enrolment deadline is enforced on the hosted page, not only on
  `AuthService.Login`, and the page can enrol a factor — QR, key, or a tap
  into an authenticator.

Operator passwords can be changed at all. `ChangePlatformPassword` for
somebody who knows theirs, `ResetOperatorPassword` for somebody who does not,
and `anubisd operators reset-password` for the case neither covers — a lone
owner locked out with nobody left to help. Before this no route changed a
platform password, so the remedy for a suspected compromise was to abandon
the account and lose its assignments.

Security fixes: a TOTP code could be accepted eight times out of eight
concurrently; an operator who exists took 51.9 ms to reject and one who does
not took 0.35 ms, making every operator username discoverable by timing; a
stolen password skipped an enrolled second factor through the browser door;
failed sign-ins through the hosted page were never audited; a scope-sync feed
could be pointed back at this server through a redirect; rotating the local
key locked out every enrolled second factor.

Schema: eight migrations, `0043`–`0050`. `internal/platform/schema` is the
schema of record and `stormddl` writes the migrations; sqlc is gone. `0049`
adds signed audit anchors, which a wholesale chain rewrite cannot reproduce.
**A v0.3.1 database upgrades with `drifted: 0`**, verified before the tag.

## v0.3.1 — 2026-09-08

A sign-out fix that changes which page people see. Sign-in resolved
slug → application → population → default; sign-out passed neither an
application nor a population, so a sign-out page configured for a population
never appeared. Plus storm v0.2.0 → v0.10.0.

## v0.3.0 — 2026-09-07

Scope at a million nodes, a console that matches the server, and a first boot
that does not need a runbook. `authorize()` is flat from 32k to 1M scope
nodes (0.045 ms → 0.059 ms); the gate snapshot dropped from 530.7 MB to
91.9 MB and from 1,014,000 to 4,245 heap objects by replacing the transitive
closure map with parent pointers.

Behaviour changes: containers default to `ANUBIS_ENV=prod`;
`GetSigninPage`/`PutSigninPage` deprecated; `UpdateApplication` refuses a
changed `slug` or `kind` instead of ignoring it.

## v0.2.0 — 2026-08-27

Identity attributes are encrypted at rest. ADR-0013 chose
`identities.attributes` as the one column worth sealing, the machinery was
built, and nothing wrote the column — so the encryption guarded an empty
string on every row. Adds RPCs, adds proto fields, and changes who can reach
the database.

## v0.1.1 — 2026-08-27

**Audit log verification was broken.** `detail` is `jsonb`, so Postgres
re-renders what it stores, and the chain hash was taken over the bytes the
writer sent — which no reader could reproduce. In a database with 21,439
entries, 21,424 reported as tampered. Verification now hashes a canonical
form on both sides; backward compatible, no migration, and that same history
verifies. On v0.1.0, `VerifyAuditChain` has been reporting alteration that
did not happen.

First release whose artefacts are signed.

## v0.1.0 — 2026-08-26

First release. Multi-tenant identity and authorization as a single Linux
binary.
