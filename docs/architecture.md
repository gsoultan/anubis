# Architecture

## Contents

1. [Design principles](#design-principles)
2. [The three planes](#the-three-planes)
3. [Component layout](#component-layout)
4. [Token architecture](#token-architecture)
5. [The scope model](#the-scope-model)
6. [Request flows](#request-flows)
7. [Consistency and caching](#consistency-and-caching)
8. [Failure modes](#failure-modes)

---

## Design principles

**1. Anubis is never in the synchronous hot path.**
Applications verify access tokens *offline* against published public keys. Anubis
can be down for ten minutes and nothing user-facing breaks — only new logins and
token refreshes fail. This property is worth more than any feature, and every
other decision defers to it.

**2. Anubis owns identity, not domain data.**
Applications keep their own profile and business data keyed by `sub`. The moment
the HR application stores employee fields in Anubis, you have a distributed
monolith and every application's schema change becomes the identity team's
problem.

**3. Applications own their permission and route catalogs.**
A team shipping a feature must not need an Anubis deploy. Catalogs are registered
by manifest. Anubis validates and stores; it does not curate.

**4. Security invariants live in the schema, not in application code.**
Anything that would be a privilege-escalation or cross-tenant bug is expressed as
a database constraint. The schema is what is still true after someone writes a
bad script at 2am. Seven such invariants are enforced and
[tested](../bench/negative.sql).

**5. Fail closed, then fail static.**
An authorization decision that cannot be fully evaluated is a denial. But a
*cached* decision surviving a database outage beats a company-wide outage, so the
gate serves from a snapshot and only fails closed when the snapshot is stale
beyond a configured age.

---

## Identity populations

Anubis serves three populations from one tenant. They differ in ways that are
**not scope questions**, so `realm` is a first-class concept rather than another
axis ([ADR-0007](adr/0007-external-identities.md)).

| | Employee | Partner | Public |
| :--- | :--- | :--- | :--- |
| Created by | HR sync | Contract onboarding | **Self-registration** |
| Factors | password + TOTP + SSO | password + TOTP | password / email OTP |
| Assurance | verified, ID on file (IAL3) | contract-verified (IAL2) | self-asserted (IAL1) |
| Administered by | IT | **their own admin** | themselves |
| Session TTL | 12 h | 8 h | 1 h |
| Retention | employment records | contract + legal | **statutory limit** |
| Access shape | org / office / department | their own company | **only their own record** |

**Partners do not get their own tenant.** A supplier user needs access to *our*
purchase-order data, which would require cross-tenant grants — and every scope
foreign key is deliberately composite on `tenant_id` to make those impossible to
insert. Partner companies are scope nodes on a `partner` axis instead, so
authorization works unchanged.

Three extra gates run in `authorize()`, all fail-closed:

1. **Identity state** — disabled or anonymised identities are denied regardless
   of grants, so deprovisioning is one field rather than a grant sweep
2. **Assurance** — a self-registered applicant cannot approve a purchase order
   *even if a misconfigured grant says so*
3. **Self-scope** — `grants.self_scoped` plus a reserved `_owner` target covers
   "only your own record" without opening the door to general ABAC

---

## The three planes

Anubis is one deployable, but three logically separate concerns:

| Plane | Question | Owns |
| :--- | :--- | :--- |
| **Identity** | Who are you? | `realms`, `identities`, `credentials`, `sessions`, `consents` |
| **Authorization** | What may you do, and where? | `scope_axes`, `scope_nodes`, `roles`, `permissions`, `grants` |
| **Token** | How do you prove it? | `signing_keys`, `refresh_tokens` |

Keeping these separate is what allows a new authentication factor to be added
without touching authorization, and a new scope dimension without touching
tokens.

---

## Component layout

Seven bounded contexts (ADR-0010), each owning its domain, its ports, its
application services and its adapters. Nothing crosses a context except
through ports and domain types.

```
                 ┌──────────────────────────────────────────────┐
  browsers ─────►│ adapter/rpc     (Connect: also gRPC/gRPC-Web) │
  mobile   ─────►│ adapter/http    (OIDC, well-known, gate)      │
  services ─────►├──────────────────────────────────────────────┤
                 │ endpoint/   go-kit middleware composes here   │
                 ├──────────────────────────────────────────────┤
                 │ service/    coarse surfaces over usecases     │
                 ├──────────────────────────────────────────────┤
                 │ app/        one usecase per operation         │
                 ├──────────────────────────────────────────────┤
                 │ port/       interfaces the context needs      │
                 ├──────────────────────────────────────────────┤
                 │ domain/     entities + rules  (stdlib only)   │
                 └──────────────────────────────────────────────┘

internal/
  identity/   who exists — identities, credentials, realms, consents
  auth/       how they prove it — sessions, tokens, sign-in, MFA, devices
  authz/      what they may do — roles, permissions, grants, decisions
  scope/      where they may do it — axes, node trees, external sync
  tenancy/    whose installation — tenants, applications, route policies
  audit/      what happened — hash-chained log
  gate/       may this request pass — snapshot read model, forward auth
  shared/     kernel: errors, principal, clock, validation
  platform/   technical: config, crypto, database, migrate, middleware
pkg/anubis/   public client SDK — zero dependencies, verifies offline
```

Each context's `domain/` imports nothing outside the standard library, and
each carries its own generated query package
(`adapter/postgres/gen`, from `db/queries/<context>`), so no package holds
another context's SQL. Enforced by `scripts/check/`.

`pkg/anubis` ships as the public client SDK: fetch keys, cache them, verify
offline, expose middleware. It is the highest-leverage code in the project —
without it every team writes their own verifier and one of them will skip the
`aud` check.

## Token architecture

Three token types with genuinely different requirements. Treating them uniformly
is the most common mistake in homegrown SSO.

| Token | Crosses trust boundary | Verified by | Format |
| :--- | :--- | :--- | :--- |
| **Access** | Yes — every application reads it | Many services, offline | **PASETO `v4.public`** (Ed25519) |
| **Refresh** | Only to Anubis | Anubis, database-backed | **Opaque** 256-bit random, stored hashed |
| **Internal state** — MFA challenge, password reset, device enrolment, back-channel logout | No | Anubis only | **`anb.local.v1`** — AES-256-GCM + HKDF-SHA256 |

Rationale in [ADR-0001](adr/0001-token-format.md).

### Access token claims

```jsonc
{
  "iss": "https://anubis.internal",
  "sub": "usr_01HXY...",           // stable, opaque, never reused, never an email
  "aud": ["billing-api"],          // WHICH services accept this — non-negotiable
  "exp": 1735689600,
  "iat": 1735689000,
  "nbf": 1735689000,
  "jti": "jti_...",                // targeted denylisting
  "sid": "ses_01HXY...",           // enables per-session revocation
  "tid": "tnt_impack",             // tenant
  "roles": ["finance.approver"],
  "scp": "invoice:read invoice:approve",
  "scopes": {                      // active scope context, one key per axis.
    "org": "01a027ff-...",         // A MAP, never fixed fields — adding an axis
    "product": "01a027fa-..."      // must not change the token format.
  },
  "realm": "employee",             // population — drives session and auth policy
  "ial": 3,                        // identity assurance level, NIST 800-63
  "amr": ["pwd", "otp"],           // how identity was proven
  "auth_time": 1735689000,         // when — enables step-up
  "epoch": 3,                      // matches identities.token_epoch
  "ver": 1                         // token format version
}
```

Two claims that look optional and are not:

- **`aud`** — without it a token minted for the HR application is accepted by the
  payments application. Classic confused deputy.
- **`amr` + `auth_time`** — these give step-up authentication for free. A
  sensitive endpoint requires `amr` to contain `otp` and `auth_time` within five
  minutes, else returns `401` with
  `WWW-Authenticate: insufficient_user_authentication`.

### Refresh rotation with theft detection

```
login              →  issue R1, family = F

refresh with R1    →  R1 := CONSUMED, successor := R2, return R2

refresh with R1    →  R1 is already CONSUMED
                   →  THEFT DETECTED
                   →  revoke entire family F and the session
                   →  force re-login, alert the user
```

If an attacker steals R1 and uses it, the legitimate user's next refresh trips
the alarm. If the user refreshes first, the attacker's use trips it. Either way
the compromise is detected rather than quietly renewed forever.

### Key rotation

`pending → active → retiring → retired`. Publish the new public key **before**
signing with it so consumer caches warm, then flip. Keep the old public key
published until the longest-lived token signed with it has expired. Rotate every
30–90 days.

`kid` may only index a **pre-loaded, bounded, in-memory map**. Never a database
query, never a filesystem path, never a network fetch. Attacker-controlled input
must not drive I/O.

---

## The scope model

Access is constrained along **independent axes**, each an independent tree.

```
org axis                product axis          customer axis        cost_center axis
  impack                  catalog               accounts             all
  ├── jakarta             ├── line-a            ├── enterprise       ├── div-1
  │   ├── finance         │   ├── sku-1         │   ├── acme         │   └── cc-1
  │   │   └── team-ap     │   └── sku-2         │   └── globex       └── div-2
  │   └── hr              └── line-b            └── smb
  └── surabaya
```

Organisation, application, department and work office are **node types on the org
axis**. Product, customer and cost centre are **separate axes**, because they do
not nest under offices — forcing them into one tree would duplicate the entire
product catalog under every office.

A grant is `(identity, role)` plus a set of per-axis constraints:

- **within an axis** — OR (`line-a` OR `line-b`)
- **across axes** — AND (org AND product AND customer)
- **axis absent from a grant** — governed by `scope_axes.default_effect`

That last rule is what makes new dimensions safe to add: an axis introduced next
year is absent from every existing grant, so nothing breaks. Measured across
2,000 sampled decisions before and after adding a `cost_center` axis:
**0 changed decisions.**

Full rules in [ADR-0004](adr/0004-authorization-semantics.md); structure in
[ADR-0003](adr/0003-scope-model.md).

---

## Request flows

### Login (password, with MFA)

```
client                          anubis                        database
  │  POST /v1/auth/login          │                              │
  ├──────────────────────────────►│                              │
  │                               │  load identity + credential  │
  │                               ├─────────────────────────────►│
  │                               │  verify KDF hash             │
  │                               │  (ALWAYS run, even if the    │
  │                               │   user does not exist —      │
  │                               │   otherwise timing leaks     │
  │                               │   user enumeration)          │
  │  202 { mfa_required,          │                              │
  │        mfa_token }            │  mfa_token = anb.local.v1,   │
  │◄──────────────────────────────┤  60s TTL, bound to device fp │
  │                               │                              │
  │  POST /v1/auth/mfa/verify     │                              │
  ├──────────────────────────────►│  decrypt + verify TOTP       │
  │                               │  create session              │
  │                               ├─────────────────────────────►│
  │                               │  issue PASETO + refresh      │
  │  200 { access_token,          │                              │
  │        refresh_token }        │                              │
  │◄──────────────────────────────┤                              │
```

### Authorization — the offline path (the common case)

```
client ──► application ──► pkg/anubis middleware
                             │
                             ├─ verify PASETO signature   (~50 µs, no I/O)
                             ├─ check aud, exp, epoch
                             ├─ resolve roles → permissions from cached snapshot
                             └─ allow / deny

   Anubis is NOT contacted.
```

### Authorization — the decision endpoint (fine-grained)

```
application ──► POST /v1/authorize
                { subject, permission, scopes: {org, product, customer} }
                             │
                             ├─ candidates: grants for this identity
                             │              carrying this permission
                             ├─ per constrained axis: is the granted node an
                             │  ancestor-or-self of the target?
                             ├─ any constrained axis unsatisfied → DENY
                             └─ any strict axis unaddressed      → DENY
```

Measured: **0.048 ms per decision, ~21,000/sec** on one connection against
150,000 grants. See [benchmarks.md](benchmarks.md).

### Securing a website path (no application changes)

```
browser ──► nginx  ── auth_request ──► POST /v1/gate/check
              │                          │
              │                          ├─ match route policy by priority
              │                          ├─ normalise path (before matching)
              │                          ├─ verify token, resolve scopes
              │                          └─ 204 allow / 401 login / 403 deny
              │
              └─ on 204: proxy to backend with X-Anubis-Subject
```

Three enforcement modes in [ADR-0006](adr/0006-path-protection.md).

---

## Consistency and caching

**`catalog_version`** is the single invalidation signal. Every table feeding the
hot-path snapshot bumps one monotonic counter per tenant, and `pg_notify` pushes
the change.

Two properties matter:

**Statement-level, not row-level.** A bulk sync of a 20,000-node customer tree
must produce **one** bump, not 20,000. Row-level triggers would serialise every
row against a single hot tuple and overflow the fixed-size `pg_notify` queue,
aborting the transaction. See `migrations/0006_statement_triggers.sql`.

**Snapshots load in one `REPEATABLE READ` transaction.** Loading eight tables in
separate queries yields a *torn read* — a grant referencing a scope node absent
from the node map, because a sync committed between query three and query four.
The result is a silent deny (or with sloppy nil handling, a silent allow), once a
week, unreproducible.

```go
tx, _ := db.BeginTx(ctx, &sql.TxOptions{
    Isolation: sql.LevelRepeatableRead,
    ReadOnly:  true,
})
defer tx.Rollback()
// every table now loads from the same MVCC snapshot
```

`LISTEN/NOTIFY` is the push path; polling every N seconds is the **correctness
backstop**, because NOTIFY is not delivered across connection drops.

---

## Failure modes

| Failure | Behaviour | Blast radius |
| :--- | :--- | :--- |
| Anubis process down | Offline verification continues | New logins and refreshes fail |
| Postgres down | Gate serves from snapshot (fail-static) | No new logins, no grant changes |
| Snapshot older than max age | Gate fails **closed** | Protected paths deny |
| Signing key compromised | **Total** — attacker mints tokens for any user | Rotate immediately, bump every `token_epoch` |
| Refresh token stolen | Detected on next legitimate refresh, family revoked | One session |
| Access token stolen | Valid until `exp` (≤ 15 min) unless `sid` revoked and app introspects | One session, bounded |
| Clock skew between services | Token rejected as `nbf` in future | Enforce NTP; allow 60s leeway |

The deliberate asymmetry: an *availability* failure degrades to stale-but-correct
answers, while an *integrity* failure denies. Never the reverse.
