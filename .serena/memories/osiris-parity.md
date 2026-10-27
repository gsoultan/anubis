# Osiris parity — what Anubis gained so the Osiris monolith can hand over auth (2026-10-26)

Osiris (`~/projects/osiris`, branch `osiris-prod`) is the Impack business
monolith that still issues the shell's session and decides permissions. The
read of its model lives in `~/projects/obelisk/OSIRIS-AUTH.md` and the obelisk
Serena memories (`osiris-auth`, `osiris-user-admin`, `anubis-already-does-iam`).
This memory is what was BUILT in Anubis to cover it, and the gaps that remain.
Branch `feat/osiris-parity`, draft PR #88.

## The shape of Osiris, from its source

Employees hold a password (bcrypt at the library's minimum cost); applicants
and registrants have none and get in only through emailed SHARE links.
Permission check is `CheckRule(ruleName)` = "holds rule X anywhere", several
hundred call sites, rules named `VERB_NOUN`, rule ← policy ← group ←
placement (org/unit/workplace/grade/jobtype). Row filtering is separate and
ad hoc: scope-picker calls and raw SQL walks over `iam.scopes`. Sharing a
record = a `common.shared` row + a 7-day SHARE session minted into an email.
Four live defects worth not copying: unauthenticated `ResetPassword` by
username; a SHARE link can mint a full login via `CreateSessions`; shared
rules apply to every record of the kind and survive unshare; bcrypt at
minimum cost. (Counts from the staging backup were deliberately left out of
this note.)

## Built

1. **Legacy bcrypt import** — `IdentityAdminService.ImportPasswordHash`,
   `internal/platform/crypto/legacyhash` (the ONLY `x/crypto` importer,
   verify-only; ADR-0002 amendment). Mandatory expiry ≤ 90 days; verified once
   at sign-in, then the row is REPLACED by PBKDF2 without the expiry (editing in
   place would hand the new hash the old deadline). Expired → audited
   `credential_expired`, told "invalid username or password".
2. **Password reset and change** — `internal/identity/app/recovery`
   (`identityrecovery`), RPCs on AuthService, hosted pages at
   `/p/{tenant}/reset/{request,complete}` in
   `internal/auth/adapter/http/recovery`. Request answers nil for everyone;
   the only error, `mail_not_configured`, is checked BEFORE any lookup (test
   counts lookups). Delivery is detached from the request so SMTP latency is
   not an enumeration oracle. Mail: `internal/platform/mail` (net/smtp,
   STARTTLS default, refuses downgrade), `ANUBIS_SMTP_*`, log sender outside
   prod, nothing in prod without a host.
3. **`AuthzService/ListGrantedNodes`** — "where, on this axis, may the subject
   do this", once per screen. Computed in `internal/gate/snapshot/granted_nodes.go`
   (mirrors Evaluate with one axis open; differential test pins agreement on
   every place), served by `internal/gate/app/granted_nodes_interactor.go`
   which satisfies `authzreach.NodesUsecase` structurally. Stale snapshot →
   `snapshot_unavailable`, never served. SDK: `Client.GrantedNodes`.
4. **SwitchScope entitlement** — it accepted any active node on the right
   axis; now refuses (`permission_denied`) unless a live grant reaches the
   place, via `authzreach.ScopeReach` over the same snapshot.

## Folder-limit moves made along the way

`internal/identity/app/retention/`, `internal/identity/domain/outbound/`
(Mail), `internal/authz/domain/granted/`, `internal/authz/app/reach/`. Each
exists because its parent was at ten files, not because the concept asked
for it; merge back if the limit ever moves.

5. **Record shares** (migration 0058, `record_shares`) — one person opens
   ONE record to another for ≤ 90 days with a subset of what they may do on
   it. `authorize()`/`authorize_explain()` gained a third allow path consulted
   only when targets name `_record` ("kind/ref"); the gate snapshot mirrors it
   (`SharesByIdentity`, `sharedRecordAllows`). `ShareService` in
   `internal/authz/app/share`: "you can only share what you can do" is the
   engine's own answer (sharer authorised through grants, `_record` stripped
   so a share cannot beget a share); revoke bumps the catalog version so the
   gate learns at once. Shares are not places: `ListGrantedNodes` and
   `SwitchScope` ignore them. Integration tests in
   `test/integration/recordshares` hold authorize() to the row.
6. **`cmd/osiris2anubis`** — reads Osiris's tables read-only and writes
   manifest.json (rule `VERB_NOUN` → `<noun>:<verb>`, policy → role),
   rules.csv for the Osiris CheckRule adapter, the people-and-access
   workbook (one grant row per person × role × axis from group membership
   at a placement; grade/job type dropped; `*_FULL_SCOPE` → everywhere),
   and a report of every decision. Applies nothing; never writes a hash.

7. **Access links** (migration 0059) — `ShareService/CreateAccessLink` puts
   a hashed link on a live share; `AuthService/OpenAccessLink` exchanges it
   for a record-bound token: ten minutes, never past the share, no session,
   amr `link`, scopes `{_record}`. Worth exactly one record, enforced where
   questions are asked: `Authorize` refuses it (`link_bound`) for any other
   subject/record (`internal/authz/app/authorize_interactor.go`), the gate
   refuses it outright (`internal/gate/app/decision.go`), `ListGrantedNodes`
   refuses it. `Principal.LinkBound()`. Anubis mails nothing — the app owns
   the record URL.
8. **Sign-in by e-mail link** — `internal/auth/app/emaillink`; realm factor
   `email_link`; second submit on the hosted sign-in form posts the same
   form to `POST /v1/login/email`; `GET /v1/login/email?token=` opens an
   ordinary browser session (amr `email_link`) and issues the code for the
   flow carried in the one-time token (`email_login` kind). Same
   no-enumeration rules as the reset flow.
9. SDK (`anubis-sdk` branch `feat/granted-nodes`): `GrantedNodes`,
   `Share`/`Unshare`/`SharesOnRecord`/`SharedWithMe`, `CreateAccessLink`,
   `OpenAccessLink`, `RecordTarget`.

## Not built, and why

- Console screens for shares, access links and "password came from Osiris".
- An API (non-hosted) e-mail sign-in door: the hosted one carries the OIDC
  request inside the link; an API door would need an application-owned
  landing URL to be configured, which is a product decision.
- The dev database on :7449 is at 0059; `stormddl -check` agrees with the
  model.

Related: [[core]], [[login-doors]], [[scope-exclusions]].
