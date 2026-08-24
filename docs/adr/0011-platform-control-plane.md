# ADR-0011 — The platform control plane

## Context

Anubis has one kind of authority: a grant, held by an identity, inside a
tenant. ADR-0008's realm model made that deliberate, and migration 0008 says
why in the strongest terms the codebase uses anywhere:

> Every grant/scope FK in this schema is deliberately composite on `tenant_id`
> precisely to make cross-tenant references impossible to insert — the single
> strongest control in the design.

That serves the people who *use* an installation. It does not serve the people
who *run* one. Whoever operates Anubis has to create tenants, and hand an
administrator responsibility for a tenant they are not a member of. Today
there is nowhere to put that authority:

- `identities_username` is unique on `(tenant_id, realm_id, lower(username))`,
  so an identity belongs to exactly one tenant by construction.
- No admin RPC accepts a tenant. Every one acts on `p.TenantID`, taken from
  the caller's own token.
- `identity_links` joins the same human across *realms*, never across tenants,
  and explicitly never merges grants.

So the installation operator has been modelled as "an admin of the first
tenant", which is not what they are. The console already half-admits this: its
sidebar carries a `WorkspaceSwitcher` and a `currentTenantId: null = Platform
view (super admin)` — a UI for a capability the server does not have.

The tempting fix is to relax the tenant FKs so a grant can point across
tenants. That is the one thing this design must not do.

## Decision

**Two planes, and only one of them is new.**

| | Data plane (unchanged) | Control plane (new) |
| :--- | :--- | :--- |
| Subject | a person who *uses* a tenant | a person who *operates the installation* |
| Authority | `grants` scoped by `scope_nodes` | `platform_assignments` |
| Decided by | `authorize()` in SQL | the control plane, in `guard.Require` |
| Boundary | tenant | assignment to a tenant |

The data plane does not change. No cross-tenant grant is ever inserted, and
the composite FKs stay exactly as they are. Operator authority is a separate
mechanism that never borrows the grant tables.

### Operators are identities in a platform tenant

One tenant is marked as the platform tenant; operators are ordinary
identities inside it. They therefore reuse password policy, MFA, sessions,
PASETO, refresh-token theft detection, retention and audit unchanged. An
operator is still "a person who can authenticate" — only their *authority*
comes from somewhere else. Building a second table of humans with a second
credential stack would have duplicated every one of those controls, and the
second copy is the one that rots.

### Authority is an explicit, expiring assignment

```
platform_assignments(operator_id, tenant_id, role, granted_by, valid_until)
```

`tenant_id IS NULL` means every tenant — the installation owner. Any other row
names one tenant, which is the "administrator assigned to a tenant" case.
`role` selects what the operator may do there (below).

### Operators act by exchanging a token, not by passing a tenant argument

`EnterTenant(slug)` verifies the assignment and mints a short-lived token with
`tid` set to the *target* tenant and a new `act` claim naming the operator.
Every existing admin RPC then works unchanged, because they already read
`p.TenantID`.

The alternative — a target-tenant parameter on each admin RPC — was rejected:
it would touch ~60 RPCs and create ~60 new places to forget the check. On a
security surface, one gate beats sixty.

### The guard branches once

An operator holds no grants in the target tenant, so `authorize()` would deny
everything. `guard.Require` therefore branches exactly once: a principal
carrying `act` is authorised against `platform_assignments` and its operator
role; everyone else takes the existing path.

This is honestly a second decision engine for the admin plane, which ADR-0010
and `AGENTS.md` otherwise warn against. It is accepted here because the
alternatives are worse:

- **Shadow identity with a real grant in the target tenant** keeps `authorize()`
  as the single decision point, but writes operator rows into customer tenants
  — corrupting who-is-in-this-tenant, headcount, retention and exports, for
  every tenant, permanently.
- **Teaching `authorize()` about operators** (migration 0013) also keeps one
  decision point, but edits the hottest and most safety-critical SQL in the
  system — the function every request on the authorize path runs, whose
  semantics `core.md` requires to be differentially tested. A control-plane
  feature must not put the data plane's hot path at risk.

The branch is confined to one function, it is audited on every call, and the
operator path never touches `authorize()` — so runtime authorization
semantics and performance are untouched.

### Operator roles, not blanket power

The assignment carries a role, so an operator is assigned *responsibility for
a tenant*, not ownership of it:

| Role | May |
| :--- | :--- |
| `support` | read tenant configuration and administer people |
| `admin` | everything an in-tenant administrator may do |
| `owner` | `admin`, plus assigning other operators to that tenant |

### Setup is an installer, gated by the config file

There is no database when an installation starts for the first time, so
first-run state cannot live in one. `anubisd` boots into installer mode when
`config.yaml` is absent: no pool, no keyring, no jobs, serving only the
console and the setup endpoint. Setup takes the database connection, tests
it, migrates it, provisions the platform tenant and its owner, and writes
`config.yaml` **last** — so a failure part-way never leaves a file that makes
an unconfigured install look configured.

The file's existence is the gate. Delete it and the installer opens again,
which is deliberate (it is how an install is redone) and also why the file
must be treated as a secret: whoever reaches an open installer chooses the
database this instance trusts.

That race is closed with a one-time token printed to the server's console at
first boot, held in memory because there is nowhere else to hold it yet. An
install wizard anyone may complete is how a fresh public deployment gets an
attacker for an owner.

The database password in the config is sealed with AES-256-GCM under an
HKDF label distinct from the keyring's, so a config ciphertext can never be
substituted for a sealed signing key. The master key comes from
`ANUBIS_MASTER_KEY` when set, and otherwise from a generated `anubis.key`
written 0600 beside the config. Be clear about what the second mode buys:
it stops a password being read over a shoulder or committed to git; it does
not stop anyone who can read the filesystem.

### Entering a tenant requires step-up

`EnterTenant` demands a fresh second factor, and the token it mints is
short-lived. Staff reaching into customer data is exactly the case step-up
exists for, and Anubis already has the TOTP and step-up machinery. A stolen
operator session must not silently reach every assigned tenant.

## Consequences

- The data plane is unchanged; the strongest control in the design survives.
- Two decision engines exist for the admin plane. `guard.Require` is the only
  place that knows, and it must stay that way — a second branch elsewhere is
  the failure mode to watch for in review.
- An operator token is not a tenant member's token. It must never satisfy
  end-user flows, and audit must record both the operator and the tenant they
  acted on, or an action inside a tenant becomes unattributable.
- The context is `internal/control`, not `internal/platform` — that folder is
  already the technical one (config, crypto, database), and ADR-0010 keeps
  contexts and plumbing apart.
- Suspending a tenant, or expiring an assignment, must invalidate tokens
  already minted for it; the short TTL bounds the window rather than closing
  it.

## Revision — tenant-side administration removed (2026-08-24)

The first draft kept a delegated-administration path: every tenant carried an
`anubis` application owning the anubis:* permission catalog and an
`anubis.admin` role, so a tenant's own person could administer their tenant
through an ordinary grant. The owner rejected that model, and the objection
is structural: it mixes the two populations this ADR exists to separate. A
tenant's roles and permissions describe what its PEOPLE may do in its
APPLICATIONS; who may administer the tenant is decided by
`platform_assignments` and nothing else.

Migration 0029 removed the machinery — the per-tenant `anubis` and `console`
applications, the anubis:* catalog rows, the `anubis.admin` role and every
grant of it — and the admin guard now refuses any non-platform principal
outright rather than consulting `authorize()`. The permission strings the
admin plane checks exist only in the operator role allow-lists
(`internal/control/domain/operator_role.go`); no tenant grant can confer
them, because the rows that could carry them no longer exist.

One capability knowingly went with it: API keys belong to tenant identities,
so nothing a tenant holds can call the admin plane any more — including a CI
pipeline applying an application manifest. If unattended manifest publishing
is wanted back, platform users need their own API credentials; that is a new
feature, not a filter.
