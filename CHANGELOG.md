# Changelog

Each release's annotated tag carries the full account — `git show v0.4.0` —
and this file is the index into them: what changed, what breaks, and what to
do about it before upgrading.

Pre-1.0, a minor bump carries deliberate behaviour changes and a patch does
not. Releases are built and signed by tag and published by hand, so a tag
existing does not mean a release was ever meant to be installed.

## v0.5.0 — 2026-10-03

Memberships can apply where each member is assigned, a structure's level
rules are held by the database, and giving access moved onto a person's page.
Three migrations, `0053`–`0055`, forward-only — **a v0.4.5 database upgrades
with `applied: 3, drifted: 0`**. A minor bump: behaviour changes below.

- **A membership can apply where each member is assigned.** A membership
  either gives every member the same places, as before, or names a structure
  and places each member in it when they are added (`0054`). A seat on the
  Marketing Council in Company A then gives nothing in Company B, and a member
  can be placed at any depth — a single work office. One person can hold the
  same membership at several places, each with its own end date and note, and
  leave one without losing the others. Which kind a membership is, is fixed
  when it is created; every existing membership keeps the old behaviour.
- **A membership's contents can change after people join.** Replacing its
  roles used to be refused once anybody had ever held it; unchanged roles now
  keep their members' grants, and the rest are added or taken back at once.
- **Membership writes now check the tenant.** An operator of one tenant could
  add another tenant's person to that tenant's membership, resync it or
  replace its contents: the membership id was taken on trust.
- **Revoking a grant keeps why it was given** (`0053`): the revoke reason has
  its own column instead of overwriting the grant's.
- **A structure's levels hold together** (`0055`). The database refuses a
  second top level, a parent that is not a level of the same structure, giving
  the top level a parent, and removing a rule items still rely on — archived
  items included. A level may sit inside itself (a company owned by a
  company). Existing rows are not re-checked; every rule applies to the next
  write.
- **Explain and the strict dry run answered with an internal error** for
  anybody holding a place-limited grant when the request named no targets.
- **The console.** Giving access is one sheet — what, where, how long, why,
  then a review — and where is an explicit choice, never a default. A person's
  access is read and changed on their page; the Access screen is gone and
  `/grants` redirects to People. Memberships has a roster, editing, and adding
  a member at a place. Structure edits levels for real (the old controls
  called an endpoint that did not exist), and renames, moves, archives and
  restores items, each saying what it does to access. A refused request shows
  the reason the server gave.

**Action required** for anything that calls the API or the database directly:

- `AssignMembership` on a membership that applies where members are assigned
  needs `scope_node_id`; without one it is refused. `UnassignMembership` by
  person and membership removes every place they hold it; send
  `assignment_id` to remove one.
- `0054` replaces the SQL functions `membership_assign` and
  `membership_unassign` with new signatures and drops the old ones. Scripts
  calling them directly must move to the new arguments.
- A new structure's top item is named after its top level — "All Partners" —
  rather than `All <axis code>`. Existing items keep their names.
- Levels and structures are still shared by every tenant on an installation,
  and `anubis:scope:admin` in any tenant edits them for all.

New, all additive (`buf breaking` is clean): `ListMembershipAssignments`,
`UpdateScopeNodeType`, `RenameScopeNode`, `RestoreScopeNode`;
`CreateScopeAxisRequest.top_level`, `Membership.anchor_axis`,
`Grant.via_assignment_id`, `ScopeNode.path`; the Import workbook's Memberships
sheet takes a place, reach, end date and note. A guard's refusal carries its
sentence as the `reason` error detail.

Internally, CI ran `./test/integration/` and nothing below it, so six
integration packages ran nowhere; it runs the whole tree now, along with the
console's unit tests.

## v0.4.5 — 2026-09-28

A security fix for machine tokens, and the audit log gains an actor filter.
One migration, `0052`, a nullable-free column with a default — **a v0.4.4
database upgrades with `applied: 1, drifted: 0`**.

- **A machine client could mint a token for any audience.** A token's `aud` is
  what stops a token for one application being accepted by another. For a user
  token Anubis sets it; for a `client_credentials` machine token the audience
  was whatever the caller sent, unrestricted — so a client authenticated as
  one application could mint a token audienced at a sibling and be accepted by
  it, defeating the control for the grant most likely to talk
  service-to-service. `applications.allowed_audiences` (`0052`) is the opt-in
  list: a client may request its own slug or a listed audience, nothing else.
  Default empty is self-only. **Action required:** an application whose machine
  tokens call another must add that audience to its `allowed_audiences`.
- **The audit log filters by actor.** Clicking an actor in the log filters to
  everything that account did — server-side on the actor id, like the action
  and result filters, so it narrows the whole log rather than a page.

Internally, the alert rules gained firing tests: `promtool` unit tests assert
each alert triggers on the series it is meant to, and a test renders
`/metrics` and checks every metric a rule names is actually exposed — so a
rule that would silently never fire is caught in CI rather than during the
incident.

## v0.4.4 — 2026-09-27

The audit log is searchable and pageable past its newest hundred entries. No
migration and no breaking change — **a v0.4.3 database needs no upgrade**
(`applied: 0, drifted: 0`), and the one proto change is an added field.

- **The Audit screen showed only the newest hundred entries.** The console
  fetched one page of a hundred and filtered the rest in the browser, so the
  search box and the allow/deny/error filter narrowed those hundred rows and
  there was no way to reach the others. On a log with a hundred thousand
  entries an investigation searched the newest hundred and was told "no
  matches" when the match was older. Filtering and paging now happen on the
  server, over the whole log: `QueryAudit` matches `action` as a
  case-insensitive substring and gains a `result` filter (added field
  `result = 7`), the cursor the handler already returned is now actually read
  back, and the console pages with a Previous/Older stack. Nothing is filtered
  in the browser, so the page count no longer lies.

## v0.4.3 — 2026-09-27

A security follow-up to v0.4.2. No migration, no proto change — **a v0.4.2
database needs no upgrade** (`applied: 0, drifted: 0`).

- **An application could manage its user's sessions.** v0.4.2 stopped an
  application enrolling an authenticator with the token a person handed it at
  sign-in; the same token still reached the rest of the self-service surface.
  An application could read the person's whole session list — every other
  application, device and IP — and sign them out of everything, or end their
  other sessions one at a time. Ownership was always checked, so this is
  disruption and disclosure rather than takeover, but it is still a relying
  party acting as the account holder. `ListSessions` and `LogoutAll` now
  require a token Anubis issued for itself (`aud` contains `anubis`, which a
  direct `AuthService.Login` produces); `LogoutSession` requires one unless
  the target is the token's own session. `Logout` (own session) and `GetMe`
  are unchanged. **Action required:** an integration that lists or ends a
  person's sessions must use a token from `AuthService.Login`, not one from
  the OIDC application flow.

## v0.4.2 — 2026-09-27

Two security fixes and one standards fix, all present in v0.4.1. One
migration, `0051`, a nullable column — **a v0.4.1 database upgrades with
`applied: 1, drifted: 0`**, verified before the tag.

- **An application could enrol an authenticator on its user's account.**
  Signing in to an application hands it an access token, and Anubis's API
  accepted a tenant token whatever application it was minted for. Enrolling an
  authenticator asked for nothing more than a session, so an application could
  register its own device key on the person's account and then sign in as
  them, with no password and in a way that survives a password change.
  Enrolling now requires a session Anubis issued for itself (`aud` contains
  `anubis`) that signed in within the last ten minutes. **Action required:**
  an integration that enrolled with the token its app received from the hosted
  page must now use a token from `AuthService.Login`, taken just before
  enrolling.
- **An old operator token still administered every tenant.** After an operator
  changed a password they believed compromised, a token issued before the
  change kept administering every tenant they cover for up to an hour. The
  tenant-administration guard checked the operator's assignments but not the
  token epoch the platform guard already compared. It checks both now.
- **The token endpoint refused to refresh.** Discovery advertises the
  `refresh_token` grant and the code exchange issues a refresh token, but
  `POST /v1/token` served only `authorization_code`. A standard OIDC client
  lost its session at the first access-token expiry. The endpoint now serves
  the refresh grant through the same rotation as `AuthService.Refresh`, so
  reuse detection holds on both doors.

Also, from the console and test work: a refresh token now records the
application it was issued to, so a rotation re-issues with that application's
audience, format and lifetimes rather than the server's own (`0051`); the
console is tested in a real browser on every CI run; the "Add a permission"
drawer explains the manifest instead of offering a form that could not
succeed; and a class of tests that silently left rows behind now clean up.

## v0.4.1 — 2026-09-25

Console fixes, every one of them present in v0.4.0. No schema, API or proto
change: **a v0.4.0 database upgrades with `applied: 0, drifted: 0`**,
verified before the tag.

- **A person's page was blank on refresh.** The console loaded its assets by
  relative URL, so a page more than one segment deep — `/identities/<id>` —
  looked for them under its own path and got the page shell back instead.
  Clicking through from People worked; a refresh or a shared link did not.
- **Add a permission could never be submitted**, and **Add person never
  offered a category** or showed the population's factors, session length
  and retention. Both drawers read their own form's values as a snapshot
  that typing never updated.
- **Tables were cut off below ~1300px.** Columns past a panel's edge were
  unreachable — at 1024px on People, Access, Audit and Applications. A table
  now scrolls sideways while it does not fit and keeps its sticky header
  when it does.
- The console works on a phone. At 375px the sidebar is a drawer, two-column
  pages stack, and nothing is wider than the screen.

Internally, storm v0.16.0 → v1.1.0, which changes no query and no schema.

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
