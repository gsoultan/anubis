# Anubis — core memory

Multi-tenant IAM/SSO backend. Postgres 18 schema is the validated engine
(`authorize()` in SQL, 0.045 ms/decision); Go application layer wraps it.

## Load-bearing decisions (see docs/adr/)
- ADR-0002: no third-party libs except infra drivers. Crypto = stdlib only.
  PASETO/TOTP/migration runner hand-written. `internal/domain` imports stdlib only.
- ADR-0008: Connect RPC (connect-go v1.20, TS runtime v2) + go-kit endpoints.
  Two surfaces: Connect for RPC, stdlib net/http for OIDC/PKCE, well-known,
  gate check, health, hosted login. One server on :7448.
- ADR-0009: all SQL in db/queries/*.sql via sqlc → internal/adapter/postgres/gen.
  No SQL strings in hand-written Go (CI-enforced).
- pkg/anubis = nested zero-dependency Go module (verifier SDK). PASETO lives
  there; the server imports it for signing.

## Layout
cmd/anubisd (serve|migrate|keys) · internal/{config,domain,usecase,port,
endpoint,adapter/{postgres,connectapi,httpapi},crypto/{kdf,localtoken,keyring,
totp},migrate,ratelimit,snapshot} · pkg/anubis · proto/anubis/v1 · db/queries
· gen/go · migrations/ (forward-only, tracked in schema_migrations
version+sha256 checksum — must stay compatible with scripts/db.sh).

## Sharp edges
- authorize() semantics: OR within axis (bool_or, migration 0013), AND across
  axes, fail-closed on strict axes and `_owner` self-scope. Any in-memory
  evaluator must match it and be differentially tested against it.
- Snapshot loads: single REPEATABLE READ read-only tx or torn reads.
- Ports 7447 console / 7448 api / 7449 db, defined in scripts/lib/common.sh.
- Refresh reuse ⇒ revoke family + session, and that event must alert.
- Dev DB via Apple Container: scripts/db.sh; bench/rebuild.sh is the DB suite.

## Application layer (shipped in this build)
Phases 0–7 implemented and live-verified; e2e suite in test/e2e (integration
tag) covers login uniformity, refresh theft (incl. successor death — the
revocations must commit OUTSIDE the failed claim tx), authorize/explain,
logout, gate decisions, rate limiting. Enforcement: scripts/check/* wired in
.gitlab-ci.yml. Bootstrap: `anubisd bootstrap` seeds tenant/realm/admin/
apps/anubis.admin role (pattern anubis:*). Gate snapshot differential parity
with authorize() still to add as an automated test.

## Scope sync from external systems
Each axis has ONE source of truth (UNIQUE tenant_id, axis_code):
kind = http | db_query | db_table, config carries url/dsn — the fetcher
(internal/repository/feed) opens the SOURCE's own connection, never the
Anubis pool. Non-obvious invariants learned by building it:
- feeds MUST be sorted parents-first in Go (repository.SortFeedParentsFirst);
  no SQL ORDER BY expresses a topological order.
- RunSync must EnsureAxisRoot first or parentless rows violate
  nonroot_has_parent.
- unreachable feed => error; an empty feed would archive the whole axis.
- only external_ref-carrying nodes are archived; manual nodes are never
  sync's to remove (verified against ~31k seeded nodes).
- migration 0021 fixed dry runs resolving parents created in the same run.
- db_table builds SQL from validated+quoted identifiers only; ADR-0009
  records that exemption (foreign schemas sqlc cannot check).

## Structure (ADR-0010, refactored 2026-08-23)
Seven bounded contexts under internal/: identity, auth, authz, scope,
tenancy, audit, gate — each with domain/ port/ app/ service/ endpoint/
adapter/{postgres,rpc,http}. Plus shared/ (apperr, authctx, clock, validate,
jsonx, txm) and platform/ (config, crypto/*, database, migrate, mw,
ratelimit). Conventions that surprise newcomers:
- FOLDER names the layer, PACKAGE clause is context-prefixed and unique
  (internal/identity/domain => package identitydomain). Deliberate: no
  import aliases, and no package shadows locals like `identity`/`session`.
- <=10 Go files per folder, CI-enforced (scripts/check/folder-size.sh).
  Outgrowing it means a missing concept: that is why authz/domain/grant,
  authz/domain/membership and auth/app/{signin,mfa,device,session,token}
  exist.
- One sqlc package PER CONTEXT (db/queries/<ctx> -> internal/<ctx>/adapter/
  postgres/gen). No generated package holds another context's SQL.
- One repository type per context over platform/database (pool, WithinTx via
  context, MapErr + column helpers). The old god-Store is gone.
- internal/api/{connect,http} is transport plumbing and must NOT import any
  context; contexts register their own routes via srv.Handle (the compiler
  caught the cycle when this was violated).
- cmd/anubisd/application.go is the composition root: every wiring decision
  lives there, nothing else knows the whole system.

## Production readiness (2026-08-23)
Runtime: request/read/header timeouts + MaxHeaderBytes + body limits + panic
recovery in internal/api/http/server.go (NO WriteTimeout on purpose — it
would cut gRPC streams; per-request deadlines do that job, streaming
detected by content-type). Pool config in cmd serve.go (MaxConns default
4xGOMAXPROCS, lifetime jitter) + statement_timeout/application_name baked
into cfg.PoolURL(). /readyz fails on stale snapshot (gate fails closed past
maxAge, so the instance must leave the LB).
Jobs (internal/platform/jobs + cmd/anubisd/maintenance.go): partitions
(boot+daily), one-time sweep, retention, key-expiry warning; each under
pg_try_advisory_lock so replicas cooperate without leader election.
DB roles: migration 0023 (anubis_owner/app/readonly). app cannot CREATE or
UPDATE/DELETE audit_log; readonly cannot read credentials/keys/pii_keys.
Gotcha: bench/rebuild.sh is DESTRUCTIVE — after it run `anubisd baseline`
then `anubisd bootstrap` or every e2e login fails with invalid_credentials.
Second factors: an ENROLLED factor is always demanded (realm allow-list
still applies). Enrol-or-deny for required-but-unenrolled is deliberately
NOT implemented (policy flip would lock out existing users).
TOTP codes are single-use (last_step in credentials.params) — tests must
wait for the step boundary, not generate a future code (skew=1).

## Auth pages (sign-in / sign-out builder, migration 0024)
auth_pages: many per tenant per kind (signin|signout), slug = URL segment at
/p/{tenant}/{kind}/{slug}; ONE default per kind (partial unique index) that
/v1/authorize and /v1/logout fall back to — not deletable/disable-able without
promoting another. Optional application_id binding (unique per app+kind) so an
app-initiated flow keeps its branding. Page choice: ?page= > app binding >
default; a missing/disabled page falls through rather than breaking sign-in.
Config is a CONSTRAINED TOKEN SET in internal/tenancy/domain/pagecfg (enums,
#rrggbb colours, http(s) URLs, bounded text) — NEVER markup, no custom_html.
Rendering: html/template + X-Frame-Options DENY + default-src 'none' CSP.
RP-initiated logout: GET /v1/logout asks (bare GET ending sessions is
reachable from any <img>), POST carries a CSRF token that ROTATES on every
render; post_logout_redirect_uri exact-matched against applications
.post_logout_redirect_uris (a SEPARATE allowlist from redirect_uris).
Gotcha: __Host- cookies need TLS, so outside prod + non-TLS requests get
un-prefixed non-Secure cookies (internal/auth/adapter/http/cookies.go) —
without this the browser flows cannot be developed or tested over localhost.
Test gotcha: e2e shares the credential rate-limit budget; enrolment/MFA calls
use retryRateLimited.

## Bulk import: people + access from Excel (provisioning context)
EIGHTH bounded context `internal/provisioning` (domain/{,row,schema} port app
service adapter/rpc). Adding a context means adding it to BOTH loops in
scripts/check/context-boundary.sh or it is silently unchecked.
Ports (internal/provisioning/port) are narrow interfaces satisfied
STRUCTURALLY by objects the composition root already builds — a.identity,
a.authz, a.scope, identityAdmin, authzAdmin — so there are no provisioning
adapters for them. Writes go through the identity/authz ADMIN USECASES, not
their repositories, so the import inherits password policy, realm/category
validation, the permission check and the audit event instead of copying any.
Two phases, deliberately: validate read-only (resolve every name, collect
EVERY issue) then write inside one tx. They cannot be merged — Postgres
aborts a transaction on its first failed statement, so "carry on and report
the next row" is impossible once writing starts. Consequence: an import
applies WHOLE OR NOT AT ALL, and a dry run writes nothing at all (no
write-then-rollback). ImportReport.Applied is what separates dry / refused /
done; Dry alone cannot.
Least privilege is PER SHEET: People => anubis:identity:write, Grants and
Memberships => anubis:grant:admin. Not membership:admin — authz gates
AssignMembership on grant:admin, and demanding a permission the write will
not check just lets a caller through to fail deeper. Dry run is gated the
same as apply (it resolves real role/scope names).
Deliberate refusals: NO password column (a spreadsheet of plaintext
passwords gets mailed around; schema_test.go enforces this). Template data
sheets ship EMPTY — a sample row an operator fails to delete is a fictional
person in a real directory; examples live on the Instructions sheet where
they cannot be imported. Grant rows fold by (realm, username, role,
valid_until, reason) into ONE grant with several scopes (OR within an axis);
mixing a scoped and an unscoped row for the same role is REFUSED, not
guessed, because the two readings differ by how much access is handed out.
Dates: only ISO accepted — 01/02/2027 is a grant that expires 11 months off.
internal/platform/xlsx is a hand-rolled stdlib OOXML codec (ADR-0002, no
excelize). Write: inline strings only (stops Excel eating "00713"), styles
part needs fills none+gray125 and a cellStyles element or readers warn.
Read: MUST convert date-formatted serial numbers (epoch 1899-12-30, cancels
the Lotus 1900 leap-year bug) or every date column imports as "46418"; reads
cells BY REFERENCE not position, since Excel omits empty cells. Header
matching is by name with a separator-stripped fallback — schema_test.go
guards that no two column keys collide once stripped.
CONSOLE GOTCHA: ui/src/routes/import.tsx talks to the REAL backend via
ui/src/lib/anubis.ts (Connect + bearer). Every OTHER route still runs on the
mock seam ui/src/lib/api/client.ts. Do not "fix" the import page to match
its neighbours — they are the ones not yet migrated.

## Console sign-in and the per-route guard (2026-08-23)
ui/src/lib/anubis.ts (real Connect client, bearer + single-flight refresh)
existed long before anything called it — there was NO sign-in, so the console
could not reach the API at all. Added ui/routes/signin.tsx + ui/stores/auth.ts.
auth.ts is deliberately NOT zustand: tokens live in lib/anubis.ts module state
because the refresh interceptor reads and replaces them outside React, and it
CLEARS the session by itself on token-theft detection. auth.ts subscribes via
useSyncExternalStore + the new onSessionChange hook rather than mirroring —
mirroring is how a shell stays painted over a dead session.
The guard is PER ROUTE (beforeLoad: requireSession), not global, and that is
load-bearing: only /import is on the real client, the other 23 screens still
read lib/api/client (the in-memory mock) and work with NO server running,
which is how the console is developed. Gating the app globally would take
that away to protect screens holding no real data. As a screen migrates off
the mock, add requireSession to its route. ?next= is clamped to a same-origin
path (rejects // and absolute URLs) or the form is an open redirect.

## Dev environment + console/backend integration (2026-08-23)
scripts/db.sh gained `seed` (up + migrate + bench/seed.sql IF EMPTY + devadmin)
and `devadmin`. dev.sh calls `db.sh seed`, so one command yields a database the
console can actually be signed into. Sign in: tenant impack, user DEVADMIN,
password anubis-dev-password (override ANUBIS_DEV_ADMIN_*; refused when
ANUBIS_ENV=prod).
WHY devadmin and not admin: bench/seed.sql creates ~57k identities and ZERO
credentials — it exists to benchmark authorize(), not to be logged into — and
`anubisd bootstrap` only creates an identity that is ABSENT, so reusing a
human-managed `admin` would print a password that does not open it.
Console data seam: ui/src/lib/api/client.ts is the ONE file screens import.
ui/src/lib/api/live.ts now serves realms, identities, identity, roles,
permissions from the real admin RPCs, mapped into the existing types.ts shapes
so no screen changed. The rest still read ui/src/lib/api/mock.ts; client.ts's
header comment is the live/sample ledger — keep it accurate.
grants() is deliberately NOT migrated: grants.tsx calls api.grants() with no
argument ("every grant"), which the admin API refuses to offer because a tenant
here holds 150k. It needs a per-person view or a paginated RPC first.
Mapping gotchas: admin API identifies a realm by CODE, console types carry
realm_id (live.ts caches the realm list to bridge); ListIdentitiesRequest uses
PAGE_SIZE not limit (passing limit silently returns the default 50);
int64 timestamps arrive as bigint and 0 means never. Role.permission_count is
NOT carried by ListRoles — live.ts returns 0, so the roles screen shows a fake
count until the proto gains the field.
The session guard is now GLOBAL (__root.tsx beforeLoad), not per-route: once
realms/identities/roles/permissions went live, a signed-out visitor would get a
wall of failed requests instead of a sign-in form.

## Console sign-in is username + password ONLY (2026-08-23)
The sign-in form has TWO fields. There is no tenant input and no "change"
link; do not add one back.
The tenant is resolved server-side and is a DATABASE fact, not configuration:
GET /v1/console-config (internal/api/http/console_handler.go, unauthenticated)
returns {tenant, issuer, setup_required} from tenancy's ConsoleTenant() ->
db/queries/tenancy/tenant.sql GetConsoleTenant: the platform tenant if one
exists, else the only tenant if there is exactly one, else NO ROW. There is no
default tenant and ANUBIS_DEFAULT_TENANT is NOT consulted here — the first
tenant is created by /setup.
The console prefers ?tenant= then the hostname's first label then this
endpoint; when all three come up empty it says the installation is not set up
and DISABLES the button rather than asking anyone to type a slug.
Invariants: the endpoint returns ONE slug, never a list (an unauthenticated
tenant enumeration is a customer roster), and it is no-store because setup
flips setup_required exactly once.
The server still needs a tenant on LoginRequest — identities_username is
unique on (tenant_id, realm_id, lower(username)), so a bare username cannot
identify a person. Resolving it is the console's job, not the user's.
Dev gotcha: this dev database holds several tenants, so `db.sh devadmin` marks
impack is_platform (only when no platform tenant exists yet) to mirror what
/setup will do. Without it the console reads the install as never set up.

## Two populations, two doors (ADR-0011, revised 2026-08-24)
PLATFORM USERS (platform_users, migration 0026) operate the installation.
TENANT PEOPLE (identities) use it. They are UNRELATED: zero foreign keys
between the tables in either direction, and it is proven in bench/negative.sql
— a tenant identity given operator authority is UNSTORABLE, not merely
refused in code. The earlier "platform tenant" idea (0025, tenants.is_platform)
is GONE; do not reintroduce it.
Consequences that keep catching people:
- A platform username is GLOBALLY unique (no tenant to scope it), which is why
  console sign-in asks for a username and password and NOTHING else. The
  sign-in form has two fields; do not add a tenant field back.
- TWO DOORS. PlatformAuthService.PlatformLogin authenticates platform_users
  and mints a PASETO with EMPTY tid and audience "anubis-platform"
  (controlapp.PlatformAudience; internal/api/connect repeats the literal
  because transport cannot import a context — platform_audience_test.go pins
  the pair). AuthService.Login is for tenant identities through a realm, and
  the sign-in PAGE BUILDER (auth_pages, 0024) is for those people, never for
  operators.
- TWO GUARDS. authz's guard now REFUSES a platform principal outright
  (p.Platform) — it has no tenant, so authorize() would take an empty tenant
  and fail as an internal error instead of a refusal. Operator authority is
  checked by controlapp.platformGuard against platform_assignments. control
  already depends on authz, so teaching authz's guard about operators would
  close the loop.
- Platform tokens have NO refresh yet: 1h TTL, console re-asks for the
  password. audit ActorKind is "platform_user", never "identity".
Dev: `db.sh devadmin` bootstraps BOTH a tenant admin in impack AND a platform
owner (same name/password, different populations). `anubisd bootstrap` gained
--platform-user/--platform-pass for that; the wizard will do it in /setup.

## Platform user management (2026-08-24)
Page /operators ("Platform users", under the Platform nav group) ->
PlatformAdminService{ListOperators, AssignOperator, RevokeAssignment} ->
internal/control (domain/port/app/service/adapter{postgres,rpc}). Gated on
anubis:platform:assign, which is in SelfPermissions so anubis.admin's
`anubis:*` pattern covers it.
DEFINITION THAT MATTERS: an operator is somebody who HOLDS a live
platform_assignment, not merely an identity in the platform tenant. Listing
the tenant's identities was the first cut and it was wrong — it showed 500
seeded users as operators. ListOperators is KEYSET-PAGED (query/page_size/page_token -> next_page_token,
total) over platform_users; OFFSET re-scans what it skips and can show a row
twice. live.ts identities() likewise walks every page instead of taking the
first 200 of 57k and calling it the directory.
The LAST OWNER cannot be revoked: an installation with no owner has nobody who
can appoint one and the only way back is the database.
Provisioning is now SHARED: internal/control/app/provision.go Provision() +
EnsureOwner() is called by BOTH cmd/anubisd/bootstrap_cmd.go (205 -> 67 lines)
and the installer. Do not fork it — it is the most security-sensitive
sequence in the system.
Installer: cmd/anubisd/install (own package; cmd/anubisd was at the 10-file
limit). runServe calls install.Run when !config.Configured(), which serves
only /v1/setup, /v1/setup/test-connection, /v1/console-config and /healthz,
prints a one-time setup key to STDOUT, and returns once setup wrote the
config so runServe boots the real API on the same port. config.Configured()
is true when ANUBIS_DB_URL is set OR config.yaml exists — a container handed a
DSN by its orchestrator is configured and must never be shown an installer.
STILL TO DO: the /setup wizard UI, and EnterTenant + the guard's operator
branch (ADR-0011) — until that lands an operator's authority is recorded but
not yet enforced at the guard.

## Platform-user concept, as the owner stated it (2026-08-24)
OWNER runs the installation: create/modify/delete TENANTS, and create
OPERATORS. OPERATOR is assigned to specific tenants and manages, in those
tenants only, the PEOPLE and the SIGN-IN PAGES. Role allow-lists in
internal/control/domain/operator_role.go encode exactly this — owner alone
holds anubis:platform:tenants / anubis:platform:assign / anubis:tenant:admin;
support is people-only; admin adds signin:admin. Tests pin it.
The sign-in page builder menu moved OUT of the Platform nav group into the
People group: it is operator work on a tenant, not installation work.
NO EnterTenant and NO token exchange — the owner rejected it. The tenant an
operator is working in is CONTEXT: the console sends X-Anubis-Tenant (set by
the header dropdown, defaulting to the first assigned tenant) and
guard.requirePlatform checks it against platform_assignments ON EVERY CALL.
That is deliberate: a revoked assignment stops working immediately instead of
when a token expires. A platform principal with no tenant header is refused.
Wiring: guard.New(authz).WithOperators(a.control, a.clock.Now) on the three
tenant-facing interactors only — identity admin, tenancy tenant admin,
tenancy page admin. The other guards refuse operators outright, which is the
safe default for anything not deliberately opened to them.
Proven: operator confined to their tenant (impack refused, importtest
allowed), operator refused platform user management, tenant identity refused
the platform plane, platform principal with no tenant refused.

## Installer (/setup) — proven on an empty machine (2026-08-24)
anubisd boots into cmd/anubisd/install when config.Configured() is false
(no config.yaml AND no ANUBIS_DB_URL), serves ONLY /v1/setup,
/v1/setup/test-connection, /v1/console-config and /healthz, prints a
one-time setup key to STDOUT, and on success returns so runServe boots the
real API ON THE SAME PORT. Verified end to end against a blank database:
key refused when wrong, unreachable DB reported not crashed, migrations run,
owner + first tenant provisioned, config.yaml written 0600 with the password
sealed, /v1/setup 404s afterwards, and a restart reads the sealed config
without reopening the installer.
Order matters and is load-bearing: migrate -> provision -> WRITE CONFIG LAST.
config.yaml existing is what says "installed", so writing it earlier would
leave a server that refuses its own installer and cannot serve either.
TWO BUGS FOUND BY ACTUALLY RUNNING IT, both fixed — do not reintroduce:
1. config.Load() rejected an empty ANUBIS_DB_URL BEFORE reading the file the
   installer had just written, so the server died immediately after a
   successful setup. The file fallback must run before that check.
2. /v1/setup/test-connection validated the WHOLE form, so the database step
   (which runs before the owner step) reported a missing owner password.
   SetupInput.DatabaseProblems() exists for exactly that; Problems() is the
   full check.
The first tenant is OPTIONAL (first_tenant_slug/name). There is no "platform
tenant" — that idea died with migration 0026.

## Console migration off sample data — state (2026-08-24)
LIVE via ui/src/lib/api/live.ts (client.ts's header comment is the ledger —
KEEP IT ACCURATE): realms, identities, identity, roles, permissions,
rolePermissions, memberships, audit, applications, tenants + createTenant/
updateTenant/setTenantStatus/tenantStats, axes, nodeTypes, scopeChildren,
scopeSearch, scopeNode, ancestorPath, signin/saveSignin.
grants + searchGrants are LIVE now. The Access screen is SEARCH-FIRST, not a
listing: AuthzAdminService.SearchGrants filters (query over username OR role
name, identity_id, role_id, source direct|membership, include_revoked) and
keyset-pages on (created_at, id) DESC. The composite cursor matters — bulk
imports give thousands of grants the same created_at, and paging on the
timestamp alone would skip or repeat rows. Scopes are fetched for the PAGE
only; fetching them for the whole result set is what made "list every grant"
impossible. The username rides on each row so the table does no per-line
identity lookup.
STILL SAMPLE DATA: authorize() (the playground; AuthorizeExplain returns an
explanation JSON whose shape the console's rich AuthorizeResponse — per-axis
verdicts, grant evaluations, ancestor paths — does not map yet; mapping it
means reading the explain format, not guessing), dashboard(),
syncSources/syncRuns/strictDryRun.
Added along the way because the console needed them and they did not exist:
GetScopeNode and ScopeAncestors (scopeNode was briefly implemented by pulling
the WHOLE axis to find one row — 32k nodes; do not do that again), and
UpdateTenant/SetTenantStatus/GetTenantStats.
ALL SIX guards are now operator-aware via guard.New(authz).WithOperators(
a.control, a.clock.Now): identity, tenancy tenant, tenancy page, authz admin,
scope admin, provisioning import. Missing one shows up as a screen that a
platform operator cannot open even though their role allows it.
Membership.member_count came from the API replacing member_ids: a membership
can hold thousands and no screen needs the roster to say "412 members".
AssignMembership is idempotent, so the add-member picker no longer excludes
existing members.

## Installation authority is NOT tenant-scoped (2026-08-24)
guard.requirePlatform checks GLOBAL assignments FIRST, before it looks at the
selected tenant. Getting this wrong deadlocks a fresh installation: an owner
creating the FIRST tenant has none selected — there are none to select — and
the earlier code refused every platform permission when p.TenantID was empty,
so the very first tenant could never be created through the console.
Order: global assignment allows -> allow; else require a selected tenant and
match an assignment that Covers() it. internal/authz/guard/platform_scope_test.go
pins all four cases (owner with no tenant, operator needs one, operator confined
to theirs, dead assignments grant nothing).

## Platform plane hardening (2026-08-24)
RATE LIMIT on PlatformLogin: 10/min per IP, 5/min per account, burst 5 —
TIGHTER than tenant sign-in on purpose (these accounts run the installation
and there are only a handful of them). It was completely absent at first:
unlimited password guessing against the owner account. Checked BEFORE the
password is looked at. Verified: 5 attempts, then resource_exhausted.
DISABLING an operator is IMMEDIATE because ListAssignmentsForOperator JOINS
platform_users and requires status='active'. The guard runs that query on
every admin call, so a disabled operator's LIVE tokens stop working at once
rather than lingering to expiry. Do not "optimise" that join away.
Disable != revoke: assignments survive a disable, so restoring somebody puts
back exactly what they had. An owner cannot disable their own account.
platform_users.token_epoch is carried in the claim but NOT enforced anywhere
— the status join and per-request assignment check cover the same ground, so
do not assume bumping the epoch does anything.
MFA for platform users LANDED (migration 0027). TOTP secret sealed with the
master key, AAD = platform_users.id, so a secret lifted into another row will
not open. totp_last_step is a MONOTONIC single-use guard: AdvanceTotpStep only
updates when the step is strictly newer, so a code cannot be replayed inside
its own 30s window — proven against the running server with a fresh challenge.
Rule follows the identity side: an ENROLLED factor is always demanded, and
there is deliberately NO required-but-unenrolled flip (it would lock out every
operator who had not enrolled, including the only owner). A CHECK constraint
makes "enrolled with no secret" unstorable. Verify is rate-limited on the same
keys as the password step. The operators page shows who has no 2FA.
Applications screen LANDED at /applications (nav: Access): a keyset-PAGED
TABLE (search on slug/name, Previous/Next, "N of total"), add (client secret
shown ONCE for server/service kinds), rotate secret, and the MANIFEST dialog
with Check-then-Apply.
applications.is_system (migration 0028): Anubis's OWN two — `anubis`, which
owns the permission catalog, and `console` — are marked system and EXCLUDED
from the tenant list. An application is a relying party: something a TENANT'S
PEOPLE sign in to. Listing Anubis's own invites somebody to edit the redirect
URIs of the thing they are editing them with, or to hand-add a permission to
Anubis's own catalog.
TWO reads, deliberately: ListApplications (paged, NOT is_system) for the admin
screen, and AllApplications (everything) for internal checks — post-logout
redirect validation must consider every registered URI, and narrowing it to
what a screen shows would silently reject valid redirects. Permissions cannot exist before their
application does — the key is namespaced by the app slug — which is why an
empty applications list made the permission form unfillable.
MANIFEST GOTCHA that cost real time: roles reference "resource:action",
NOT the full "app:resource:action" key, and the role name is prefixed with
the app slug automatically (name "clerk" -> role "billing.clerk"). The error
now says so explicitly instead of just "invalid argument" against a value
that looks correct.
STILL MISSING for production, in priority order:
1. The access-check playground is still sample data — it will confidently
   show WRONG explanations of why access was denied.
2. Platform tokens still have no refresh (1h, then re-login).
3. dashboard(), syncSources/syncRuns/strictDryRun still sample data.

## Dev script owns the dev operator's second factor (2026-08-24)
`db.sh devadmin` CLEARS any enrolled TOTP on the dev operator before printing
its password. This is not laziness: enrolling 2FA on devadmin while testing
locked the whole dev environment behind an authenticator nobody still had, and
the printed password became a lie. The script guarantees the account it owns
can be signed into.
To exercise MFA, enrol from the console and keep the authenticator — do not
enrol on devadmin and expect the script to leave it alone. cmd_devadmin
already refuses to run when ANUBIS_ENV=prod, which is what keeps this from
being a way to strip a factor anywhere real.

## Pagination: where it exists and where it still does not (2026-08-24)
KEYSET-PAGED (cursor stack in the screen, Previous/Next, filters reset the
stack — a cursor from the old result set silently skips rows in the new one):
  /identities  people          — 57k rows; was pulling up to 2000 and rendering
                                 them, which is a wrong answer dressed as a
                                 slow one
  /grants      access search   — 150k rows; search-first, cursor (created_at,id)
  /applications                — search on slug/name, "N of total"
  /operators   platform users  — search, "N of total"
NOT paged, and knowingly so: realms and axes (a handful per tenant), roles
(117) and permissions (221) — bounded by the catalog, not by tenant size.
Revisit if a tenant ever defines thousands.
STILL UNBOUNDED and worth fixing: grants.tsx and memberships.tsx each build an
"all scope nodes" lookup by calling scopeSearch('') for EVERY axis — that is
32k nodes in this installation, fetched to render a name beside a grant.

## Control plane leaks out of tenant lists — closed (2026-08-24)
The tenant's Roles & permissions screen was showing `anubis.admin` and the
anubis:* catalog. Same category error as the applications list, same fix, same
shape: ListRoles and ListPermissions now filter on the OWNING APPLICATION's
is_system — NOT on roles.is_system, because manifest roles (billing.clerk) are
also is_system and must still appear. Verified: roles 117->116, permissions
221->204, zero anubis:* leaking, and operators still work (their authority
never came from those rows).
Platform roles were ALREADY separate tables — the owner's demand holds by
construction: support/admin/owner live in the platform_assignments.role CHECK
plus allow-lists in internal/control/domain/operator_role.go, and are NOT rows
in `roles`. Verified: 0 grants held by any platform_users id (FK makes it
unstorable — negative.sql case 9 pins it), 0 platform role names in `roles`.
anubis.admin STAYS in the roles table (hidden): it is the delegated-admin
path for a TENANT'S OWN administrator — a tenant person granted anubis.admin
can administer their own tenant. That is ADR-0011's delegated administration,
distinct from platform operators.

## Administration is operator-ONLY — delegated admin removed (2026-08-24)
Migration 0029 deleted, from every tenant: the `anubis` + `console` system
applications, the anubis:* permission catalog, the anubis.admin role, and all
grants of it. applications.is_system (0028) is gone too — nothing can create
a system application, so the marker had nothing left to mark.
guard.Require now REFUSES any non-platform principal outright (hint:
"administration is performed by platform users") — it no longer consults
authorize() at all, and guard.New() takes NO arguments (fails closed until
WithOperators). Six interactor constructors lost their authz param; scope
admin KEEPS an authz repo but only for StrictDryRun simulation, not the guard.
Provision() now creates tenant + internal realm + optionally ONE ORDINARY
PERSON (no role, no grant — realm.MinAssurance). bootstrap --admin-user now
makes that plain person; db.sh says "no admin power". The installer's first
tenant gets NOBODY in it.
Tenant login with ClientId "" mints for self — the console application is
gone, so ClientId "console" is now invalid_client (e2e fixed accordingly);
the console client's tenant login/verifyMfa/logout functions were deleted.
KNOWN capability loss, deliberate: API keys belong to tenant identities, so
CI manifest-apply via API key is dead. Bringing it back = API credentials for
platform users, a new feature.
Proven live: tenant person with a valid token -> permission_denied with the
hint; operators unaffected; roles 116 / permissions 204 with zero anubis:*;
import tests now model operator roles (support = people only, admin = +grants)
— the harness speaks assignments, not permission maps.

## API keys are the TENANT's, not a person's (migration 0030, 2026-08-24)
New `api_keys` table: tenant_id FK, created_by -> platform_users (SET NULL),
lookup + sha256-hex secret_hash, same anb_live_<lookup>.<secret> wire format.
'api_key' REMOVED from the credentials kind CHECK — a person holding one is
unstorable (negative.sql case 10). Existing rows migrated across; the
identity linkage was the mistake and is not carried.
Ownership: storage in the AUTH context (authport.APIKeyRepository, authpg);
admin RPCs on TenantAdminService (List/Create/RevokeApiKey), gated on
"anubis:apikey:admin" — in the ADMIN operator allow-list beside
application:admin (support gets neither); UI panel on /applications.
The interceptor authenticates a key as the TENANT'S SYSTEM: Principal
{IdentityID: <key id>, TenantID, Service:true} — no person, no epoch, and the
tenant's own status gates it (suspended tenant => keys stop). Still NOT
platform => still refused on the admin plane; keys are for the tenant-plane
decision API. Revocation is immediate (lookup index is WHERE revoked_at IS
NULL). GOTCHA: secret.NewAPIKey's lookup ALREADY carries "anb_live_" — do not
re-prefix (shipped doubled once).
Found while proving it: Authorize with NO scopes field marshalled nil ->
jsonb_each_text(null) -> 500. Fixed: nil scopes => {} so a machine caller
always gets a DECISION, never an internal error.

## CI is real now (2026-08-25) — it was a mirage before
.gitlab-ci.yml had been ONLY the Auto-DevOps/SAST template; commit 6016b02's
message claimed a pipeline that its diff never contained, so every
"CI-enforced" claim was aspirational. Now .github/workflows/ci.yml (origin is
GitHub) AND .gitlab-ci.yml both run: buf lint, all six scripts/check/*, vet +
build + unit with -race -shuffle=on for BOTH modules, the backend suite,
console typecheck+build, and a docker build.
scripts/ci/backend-suite.sh is the shared heart: fresh scratch DB -> migrate
-> bootstrap BOTH populations -> real server -> integration + e2e + fuzz
smoke + load. Gotchas paid for once:
- ANUBIS_ISSUER must point at the suite's own instance; page URLs and iss are
  built from it, so otherwise tests probe whatever is on :7448.
- -count=1 is mandatory: these suites hit a live server, which the test cache
  cannot see, so a cached "ok" is a lie.
- DROP DATABASE silently FAILS while any server holds a connection. A suite
  run against leftover rows then fails in ways unrelated to the code (an hour
  went into exactly this). Check the DROP printed DROP DATABASE.
- The 30s fuzz smoke found a REAL gate bug on its first run (see below), so
  that budget stays.

## Playground + ALL console writes live (2026-08-24)
live.authorize() = Authorize (verdict + step-up detail) PLUS Explain in
parallel; the per-grant tree maps 0020's exact keys: grants[].{grant_id, role,
via_role, self_scoped, self_ok, axes[].{axis,satisfied,nodes[].{node_id,node,
inherit}}, axes_ok, strict_missing, allowed}. HONESTY RULES baked in: matched
is never claimed (explain does not say which granted node matched), target
names resolve via GetScopeNode (else a supplied target renders "(not
supplied)"), strict_missing surfaces as failing gates, nil scopes were fixed
server-side earlier. Verified live: org+product grant with one target -> deny
with per-axis (org true, product false); both targets -> allow. AND-across-
axes rendering is now truthful.
EVERY Create drawer commits real rows: identity (realm/category CODES mapped
from ids in live.ts), grant+revoke, membership (+SetMembershipEntries),
assign/unassign (grants_created/grants_revoked fields), role create/update
(permission KEYS travel as patterns — a key as a pattern matches itself),
scope node/axis/node-type, sync sources, runSync dry/apply, strictDryRun.
Drawer notifications now use their own INPUTS — the mock's return values were
the old world.
NO RPC EXISTS, seam throws a pointed error instead of pretending:
createPermission (manifests own the catalog), deleteRole, setNodeTypeParents.
syncRuns() returns [] (no run history server-side); mock's syncPlan/
strictDryRun fantasy shapes DELETED — SyncPlan/StrictDryRun types now mirror
the server's actual reports (counts + errors; sampled/would_deny/examples).
STILL SAMPLE DATA: dashboard() alone (needs per-realm counts no RPC gives).


## Production-readiness pass (2026-08-25)
Everything below was built or proven in one sweep; the plan's phases 0-3.

GATE BUG FOUND BY FUZZING, do not reintroduce: /0%23 normalised to /0# — a
decoded '#' or '?' is data to this normaliser and a DELIMITER to anything
that re-parses, and normalising twice gave different answers. Both now
reject as ambiguous. The fuzz invariant was ALSO wrong: "no '..' substring"
flagged /..0, but a segment merely CONTAINING dots is a legitimate name. The
property is segment-level: no '.'/'..' SEGMENT survives.

CONSOLE IS EMBEDDED: ui/embed.go go:embeds ui/dist into the binary;
internal/api/http/console_assets.go serves it at / on the API origin (so
production needs no CORS). API prefixes (/anubis.v1., /v1/, /p/,
/.well-known/) never reach the SPA. The shell is written DIRECTLY, not via
FileServer — http.FileServer 301s /index.html to /, which turns the SPA
fallback into a redirect loop. dist/index.html is a committed PLACEHOLDER so
go build works without bun; scripts/build.sh and the Dockerfile overwrite it.

GUARD: global authority means ANY tenant, not NO tenant. A global owner with
no tenant selected now gets no_tenant_selected (failed_precondition) on
tenant-scoped calls instead of an internal error from an empty tenant id.
controldomain.InstallationScoped names the exemptions (platform:tenants,
platform:assign, tenant:admin, key:admin). The console also resolves the
default tenant in __root beforeLoad AND holds tenant-scoped requests at one
choke point in lib/anubis.ts until it is known.

METRICS: hand-rolled Prometheus text format (ADR-0002) at /metrics on the
DEBUG listener. Families: endpoint requests+duration histogram, audit events
by action (token.reuse_detected is the pager), job runs, per-tenant snapshot
loaded-at, pool gauges, build info. docs/alerting.md pairs each rule with the
runbook section AND names what is deliberately not alerted. main.version now
exists — the Dockerfile had passed -X main.version since day one with no
variable to land in.

PLATFORM SESSIONS ROTATE (0031): single-use refresh, family revocation on
reuse (successor dies too), ABSOLUTE 12h family lifetime — rotation never
extends it. Audited as token.reuse_detected with plane=platform.

PLATFORM API KEYS (0033, no 0032 — withdrawn): a key acts AS its owner and
carries exactly their assignments, re-read per call, so it can never exceed
them and dies when they are disabled. expires_at NOT NULL, capped 90 days.
This is how CI applies a manifest again after 0029.

CONSOLE IS 100% LIVE: dashboard() was the last mock; ui/src/lib/api/mock.ts
is DELETED. Also removed the console's own inventions — hard-coded trend
deltas and sine-wave sparklines beside real counts, a membership with 412
members rendering "Nobody yet.", and playground presets that looked up a
fictional "alice".
UNBOUNDED LOOKUPS ARE GONE: GetScopeNodes(ids) replaces pulling 32k nodes to
label chips; IdentityPicker searches the server instead of loading 2,000 of
57,000 people into a dropdown; Populations counts are computed in SQL (they
were tallied from a capped fetch, so every figure was WRONG).

SYNC HISTORY was never missing — scope_sync_apply has written scope_sync_runs
since 0017 and nothing read it. ListSyncRuns joins through the source for
tenant scoping (the runs table has no tenant_id).

LOAD: TestAuthorizeUnderConcurrency. One instance saturates at ~11.8k
decisions/s; 32->256 concurrent callers leaves throughput FLAT while p50 goes
2.6ms -> 21ms. Scale out, not up.

DECISIONS WRITTEN DOWN: ADR-0012 (rate limits stay per-instance; the value is
in the KDF, not the ceiling; trigger conditions listed), ADR-0013 (seal
identities.attributes ONLY — identifiers are lookup/join keys), ADR-0014 (no
CAPTCHA on registration; a self-registered account has no grants, so the loss
is noise), docs/enrolment-rollout.md (the missing piece is a grace period,
not a check; 52,004 users here would be locked out by a naive flip).
TWO DOCS WERE WRONG about controls that never existed: security.md claimed
rate-limit "counters in Redis" and api.md claimed nonce GETDEL via Redis.
There is no Redis; counters are in-process (which is WHY they are
per-replica) and nonces use DELETE ... RETURNING.

## Shipping as a Linux binary (2026-08-25) — NOT Kubernetes
.goreleaser.yaml builds linux amd64/arm64 + tar.gz + .deb/.rpm (nfpm).
scripts/build-console.sh is a before-hook that builds ui/dist AND greps for
the placeholder: the binary embeds ui/dist, so a failed console build would
otherwise ship a "console not built" page silently. CI runs a snapshot on
every push and greps the binary; release.yml builds on a v* tag after
re-running the gates.
systemd unit (packaging/): Type=exec — anubisd does NOT speak sd_notify and a
notify unit that never notifies hangs until timeout. Master key arrives as a
CREDENTIAL (LoadCredential -> ANUBIS_KEY_FILE=%d/master.key), not an env var.
ProtectSystem=strict means the INSTALLER cannot run under the unit (it writes
config.yaml) — fine, because ANUBIS_DB_URL is set so it never runs.

BUGS FOUND BY ACTUALLY INSTALLING/RUNNING, all fixed:
1. config.Load() read the master key from ANUBIS_MASTER_KEY ONLY, so prod with
   a valid key FILE refused to boot. Now either; and a source that is
   CONFIGURED-but-broken is fatal even in dev (falling back to the printed dev
   key would seal prod data under it).
2. A fresh prod install has NO signing key (AutoKeys off outside dev):
   /readyz 503, every login fails. `anubisd keys init access` mints+promotes,
   refuses if an active key exists. Step 4 of the runbook.
3. ~1 API KEY IN 9 COULD NEVER AUTHENTICATE. Keys are anb_live_<prefix>_<secret>
   with an 8-char base64url prefix — an alphabet containing '_'. SplitAPIKey
   searched for the FIRST '_', so a prefix containing one parsed as malformed.
   Measured 214/2000 = 10.7%. Affects TENANT keys (0030) as shipped, not just
   platform keys. Fixed by splitting at the known offset; backward compatible.
4. Behind a TLS proxy the client IP was the PROXY on every request, so per-IP
   rate limits bounded the WHOLE INSTALLATION — one attacker locks out
   everyone. ANUBIS_TRUSTED_PROXIES names CIDRs whose X-Forwarded-For is
   believed; header read RIGHT-to-left taking the first untrusted hop (so a
   client prepending a hop cannot choose its identity). MUST be set behind a
   proxy.

CI IS REAL AND GREEN (first run ever: 2026-08-25). It found #3 by passing
then failing on the SAME commit — the signature of a probabilistic bug, not a
flaky test. Two CI-specific pins learned: protoc-gen-go must be v1.36.11 (the
version that produced the committed bytes; 1.36.12 rewrites only the header,
enough to fail drift), and buf.gen.yaml uses `local:` plugins so CI must
install protoc-gen-go AND protoc-gen-connect-go or gen-drift cannot run at
all. Load-test latency budget is env-dependent (ANUBIS_LOAD_BUDGET_MS): a
shared runner does ~1,100 decisions/s vs ~11,800 here. Correctness assertions
(zero errors, no deny-flips) are NOT relaxed. Fuzz -parallel is bounded or an
oversubscribed host times a worker out and reports a finding it never made.

DRILLS PERFORMED (were only ever procedures):
- Restore: pg_dump -Fc + pg_restore. Schema, data, revocations and CHECK
  guards all survive; /readyz 200 afterwards, which means the restored SEALED
  keys opened — the same fact that makes a stolen backup worthless.
- Soak: ~300k decisions, goroutines flat at 17, RSS returned to 26MB from an
  83MB peak. No leaks. Throughput from that run is NOT recorded: host was at
  load average 35, and a timing on a busy machine is not a measurement.
- Two instances, one DB: each boot job ran on exactly one (advisory locks,
  other says skipped); a key revoked on A was refused by B on the NEXT
  request; rate limits confirmed per-instance (A limited, B unaffected).

Migration 0034: identities.attributes CHECK (= '{}'). ADR-0013 marks it for
sealing, the write path does not exist, every row is empty (0 of 57,012), so
plaintext there is now UNSTORABLE until somebody drops the constraint — which
forces them to read the ADR. There is no 0032 (withdrawn; sync-run history
already existed since 0017).

## The audit chain did not work (found and fixed 2026-08-27)
migrations/0005 claims "an attacker with UPDATE rights on this table still
cannot silently rewrite history". VerifyChain existed; nothing had ever
tampered with a row to check. It was BROKEN: `detail` is jsonb, so Postgres
re-renders what it stores (space after every colon, keys reordered), while
the hash was taken over the bytes the WRITER sent. 21,424 of 21,439 entries
in the dev database reported as tampered — a verifier nobody would keep
believing.
FIX: chainHash hashes canonicalJSON(detail) — parse and re-marshal, so both
sides agree whatever jsonb did to the formatting. Backward compatible with
no migration BECAUSE current writers already emit compact key-sorted JSON,
so canonical(stored) == the bytes the old writer hashed.
TWO THINGS THE REAL DATA TAUGHT, neither guessable:
- A detail-less entry has TWO historical readings: early call sites passed
  {} and later ones passed nil, and jsonb stores {} either way. Verification
  accepts both, narrowly, or history written before they were reconciled
  cannot verify. Proven by reconstructing seq 302's hash both ways.
- canonicalJSON maps empty AND {} to nothing, because storage cannot tell
  them apart.
LIMITS, now in docs/security.md: rewriting an entry AND every entry after it
is self-consistent (the defence is that it is no longer silent — anchoring
the head hash outside the database is what would close it), and the first row
of a queried range anchors the walk, so narrow ranges check less.
TEST SAFETY, learned by breaking it: the delete-detection test removes a row,
which breaks that tenant's chain PERMANENTLY. Pointed at the installation's
first tenant it destroyed real evidence in the dev database (77 probe rows +
a gap; repaired by deleting exactly the probes, which were all above the last
real seq). test/integration/auditchain now creates and drops its own tenant.

## grep -q under pipefail is a trap (2026-08-27)
`cmd | grep -q PATTERN` with `set -o pipefail` FAILS WHEN THE PATTERN IS
FOUND: grep exits at the first match, the writer takes SIGPIPE, pipefail
propagates it. It passes on short output and fails on long, so it presents as
a flake. Found in three places: scripts/lib/common.sh container_exists (could
report a running container as absent), scripts/ci/local.sh's DROP DATABASE
guard, and ci.yml's console-embedded check (safe only because the default
Actions shell omits pipefail). All now read the whole stream — count, or
capture then test.

## Release state (2026-08-27)
v0.1.1 published and verified end to end: cosign verify-blob on
checksums.txt returns Verified OK against the workflow identity, that
checksum matches the downloaded artefact, the SBOM lists the real tree
including github.com/gsoultan/storm, and the shipped binary reports 0.1.1
with the real console (zero placeholder markers) and the canonicalJSON fix.
v0.1.0 is still public and carries the broken verifier.
scripts/ci/local.sh runs the whole pipeline offline — written when GitHub
refused every job over account billing, which is also why the container-image
job now runs only on main.
## The audit log had two holes, both silent (2026-08-27)

Found while reading CI output that had been printing the first one for weeks.
Both were invisible from inside the running system, which is what made them
last.

**1. Anything filed under no tenant was discarded at the door.**
`ChainedAuditor.Emit` opened with `if ev.TenantID == "" { return }` — before
the metric, before the queue. `audit_log.tenant_id` is `NOT NULL` and every
index leads with it, so a tenant-less event genuinely cannot be stored; the
early return was correct about the constraint and catastrophic about the
consequence. A platform user belongs to no tenant (ADR-0011), so the ENTIRE
platform plane went unrecorded: every administrator sign-in, every failed
sign-in, API keys, logout, MFA enrolment — and `token.reuse_detected`, which
alerting.md calls the highest-signal event in the system. On the dev database:
**0 `platform.login` rows against 320 `auth.login` rows.**

Fixed by giving the installation a tenant id of its own —
`auditdomain.InstallationTenant`, the nil uuid, which uuidv7 cannot produce —
so it gets a hash chain of its own, which is right for a separate population.
`anubis:audit:read` joined `installationPermissions` so an operator with no
tenant selected can read and verify it through the ordinary audit API.

**2. `target_id` is a uuid, and codes were being passed to it.**
Scope axes and node types are identified by a code, not an id. Four call sites
in `scope_admin_interactor` passed the code, the insert failed on every one,
and the auditor logged it and carried on. On the dev database: **0 rows for
`scope.axis_create`, `scope.node_type_create`, `scope.axis_update`,
`scope.bulk_upsert`** — i.e. structural authorization changes were never
recorded. Codes now go in `detail`.

**The lesson worth keeping:** an audit write must never fail the request that
triggered it, so it can only be dropped — which means the drop has to be
*loud*. `metrics.IncAuditDropped(action)` now counts every one, alerting.md
pages on any increase, and `TestAdminActionsLeaveAnAuditTrail` /
`TestFailedPlatformLoginIsRecorded` fail if either hole reopens. Both were
verified to fail against the old code before being trusted.

## identities.attributes is sealed for real now (ADR-0013, 2026-08-27)

Migration 0034 had made a non-empty value unstorable, as an honest placeholder
for "this promise is not kept yet". 0035 replaces it with a constraint that
admits `{}` or `{"v":1,"sealed":"…"}` and still refuses plaintext.

`SetIdentityAttributes` seals the WHOLE map as one blob — field names included,
because `diagnosis` in a dumped column is itself the disclosure. AAD is
`"attributes:"+identityID`, so a ciphertext copied onto another row stops
opening; that, not breaking AES, is the realistic attack on per-row keys. The
per-identity key is sealed under the master key with kid `"pii:"+identityID`,
which lets it be minted in one statement.

Writes replace rather than merge — a partial update cannot express "delete
this field", and a field outliving an erasure request is the failure the
column exists to prevent.

**Sharp edge:** `pii_shred` deletes the key row and the FK nulls
`identities.pii_key_id`, so an erased identity is byte-identical to a fresh
one except for the orphaned ciphertext still in the column. The first version
happily minted a new key and resurrected it. `isErased()` — non-empty envelope
with no key — is the signal, and both the read and write paths use it.

## Scope sync reaches any SQL engine, proven per push (2026-08-27)

`feed.Dialect` holds what differs per engine (quoting, how NULL parents sort,
DSN translation); `cmd/anubisd/syncengines` is the only place a driver is
imported. Adding one is two lines there.

CI runs a `mysql:8` service on both pipelines, because without one the
multi-engine tests skip in silence — which reads exactly like passing. That
was not hypothetical: the first MySQL probe read a table a person had typed
into a container by hand months earlier, and failed the moment CI got a server
of its own. Both MySQL tests now build their own fixture, and each names a
child whose primary key sorts AHEAD of its parent so ordering is decided by
the dialect rather than by luck.

## Enrol-or-deny shipped as a date, not a flag (2026-08-27)

The last gap docs/roadmap.md admitted to. A realm could list `totp` in
`required_factors` and still admit a password-only login from someone who
never enrolled one.

Why it stayed open, and why the fix is shaped this way: closing it with a
boolean is a lockout event, not a config change. Every unenrolled member loses
access the instant it flips, and `BeginTOTP`/`ConfirmTOTP` take their
principal from the session — the exact thing the policy withholds. So:

- `realms.factor_enrolment_deadline` (migration 0036). NULL = not in force,
  which is every existing realm, so an upgrade changes nothing.
- Before the date: tokens **and** `enrolment_due` (factors + deadline).
- On/after: `enrolment_required` with a **grant token** instead of a session.
- `BeginTotpEnrollment`/`ConfirmTotpEnrollment` accept `grant_token` in place
  of a session. 15 min, no scopes, no session id, one purpose.

**Sharp edge:** a grant is minted only for a member with NONE of the required
factors enrolled, and `caller()` refuses it (conflict) if a totp credential
exists by redemption time. Without that, a leaked grant *replaces* an
authenticator rather than adding the first — account takeover wearing a
compliance hat. `TestAGrantCannotReplaceAnEnrolledFactor` holds that line.

Domain rule is `Realm.EnrolmentStanceFor(enrolled, now)`, unit-tested for the
edges that decide a lockout: `password` is not enrolable, a factor the realm
no longer *allows* is not demanded (unsatisfiable policy), and the deadline
instant itself enforces.

**Test-suite lesson:** these passed alone and failed in the suite — the per-IP
limiter had been spent by earlier tests. `retryLimited`/`signIn` in
`test/e2e/e2e_test.go` are the fix. A test that reads 429 as a fault teaches
whoever sees it to weaken the limit.

## Database privilege was described, not enforced (2026-08-27)

`migrations/0023` set up least privilege and granted `USAGE ON SCHEMA public`
explicitly to `anubis_app` and `anubis_readonly` — while leaving PUBLIC's
implicit grant in place. So those grants described an access path rather than
controlling one: every role in the database reached the schema regardless.

Found by diffing a freshly migrated database against the development one with
`pg_dump --schema-only`. They were identical except for a single
`REVOKE USAGE ON SCHEMA public FROM PUBLIC` that had been applied here by
hand and never made it into a migration. That the dev database had been
running with it is the evidence it is safe. Now `migrations/0037`.

**Upgrade note that matters:** any role added later needs
`GRANT USAGE ON SCHEMA public` BY NAME. An integration that used to work by
default now gets `relation "…" does not exist`. Documented in operations.md.

**Do not overclaim it:** `pg_catalog` stays world-readable, as in every
Postgres, so an ungranted role still lists table names through `pg_tables`.
It closes the data, not the catalog.

The other privilege claims in operations.md all held when measured, and are
now `TestDatabaseRolesCannotExceedTheirPrivilege` so they keep holding —
verified by granting `UPDATE ON audit_log` and `SELECT ON credentials` and
watching the test name both.

Related: the suite now reads `anubis_audit_dropped_total` after every run and
fails on any non-`authorize` drop. See [[audit-silent-drops]] reasoning in the
audit section above.

## The gate's fail-closed promise was untested (2026-08-27)

`internal/gate/app` had no test file at all, while operations.md promised that
past `ANUBIS_SNAPSHOT_MAX_AGE` the gate refuses and `/readyz` pulls the
instance from the balancer. The gate answers entirely from the snapshot, so a
freshness check that stopped working would serve authorization from
arbitrarily old data — honouring revoked access, at p99 under a millisecond,
with no error anywhere. Fail-static is the feature; unbounded fail-static is
the outage nobody notices.

Covered at both levels now. `GateHandler` takes a `Snapshots` interface rather
than the concrete `*gateapp.Manager` — `HealthHandler` already took a probe
for the same reason, and the concrete dependency is precisely why this branch
was untestable without a database.

**Method note worth keeping:** the first handler test was worthless. It
asserted `403` on a stale snapshot, passed, and passed again with the
freshness check deleted — an empty snapshot denies everything anyway. It
asserts the refusal REASON now, and the deleted check shows up as
`403 "no route policy"`: the gate deciding from stale data rather than
refusing to look. On a security boundary, a test that cannot tell
refusing-to-decide from deciding is not testing the boundary.

## Dependency policy is now enforced (2026-08-27)

`scripts/check/deps.sh` fails on a direct requirement outside the ADR-0002
allowlist, an import of third-party crypto, or an untidy `go.mod`. Every
pipeline globs `scripts/check/*.sh`, so it runs everywhere without wiring.

The tidy check earns its place: `go.mod` marked `go-sql-driver/mysql`
`// indirect` while `cmd/anubisd/syncengines` imported it directly. A file
that misreports which dependencies are direct cannot be used to review them.

Two false positives to remember if editing it: the `require (` line parses as
a dependency named `require`, and the crypto regex matches Anubis's OWN
`pkg/anubis/paseto` — the stdlib implementation the rule exists to require.
