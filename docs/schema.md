# Database schema

PostgreSQL 18+. Migrations in `migrations/`, applied in filename order.

**27 base tables · 40 foreign keys · 46 check constraints · 116 indexes · 11
domain functions.** Counts verified against a live instance.

---

## Contents

- [Conventions](#conventions)
- [Tenancy and applications](#tenancy-and-applications)
- [Realms and identity](#realms-and-identity)
- [Sessions and tokens](#sessions-and-tokens)
- [Dynamic scope](#dynamic-scope)
- [Authorization](#authorization)
- [Route policies](#route-policies)
- [Operations](#operations)
- [Functions](#functions)
- [Enforced invariants](#enforced-invariants)

---

## Conventions

Four rules applied throughout. Rationale and measurements in
[ADR-0005](adr/0005-database-performance.md).

1. **Primary keys are `uuidv7()`, never `uuidv4()`.** Time-ordered, so inserts
   land on the rightmost B-tree page — no random page splits, far less WAL.
2. **Column order is 8-byte → 4-byte → 1-byte/varlena**, removing alignment
   padding.
3. **Hot-update tables set `fillfactor < 100`** so updates stay on-page (HOT) and
   skip index maintenance.
4. **Partial indexes wherever a predicate is always present** (`WHERE revoked_at
   IS NULL`).

---

## Tenancy and applications

### `tenants`
Top-level isolation boundary.

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `uuid` PK | `uuidv7()` |
| `slug` | `text` UNIQUE | `^[a-z0-9][a-z0-9_-]{1,62}$` |
| `status` | `text` | `active` \| `suspended` \| `archived` |
| `settings` | `jsonb` | |

### `applications`
Relying parties. **Own their permission and route catalogs** — a team shipping a
feature must not need an Anubis deploy.

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `uuid` PK | |
| `tenant_id` | `uuid` FK | |
| `slug` | `text` | unique per tenant |
| `kind` | `text` | `spa` \| `native` \| `server` \| `service` |
| `client_secret_hash` | `text` | |
| `redirect_uris` | `text[]` | **Exact match only.** No wildcards, no prefixes — open redirect is account takeover. |
| `backchannel_logout_uri` | `text` | |
| `access_token_ttl` | `interval` | default 10 min |
| `refresh_token_ttl` | `interval` | default 30 days |
| `token_format` | `text` | `v4.public` \| `jws.eddsa` — the [ADR-0001](adr/0001-token-format.md) hedge |
| `manifest_version` | `int` | |

Composite-FK targets: `UNIQUE (id, tenant_id)`, `UNIQUE (id, slug)`.

---

## Realms and identity

### `realms`
Identity **populations** inside a tenant. Employees, partners (suppliers,
vendors, contractors) and public users (applicants, customers) differ in
authentication policy, assurance, administration and retention — none of which is
a scope question. [ADR-0007](adr/0007-external-identities.md).

| Column | Type | Notes |
| :--- | :--- | :--- |
| `kind` | `text` | `employee` \| `partner` \| `public` \| `service` |
| `min_assurance` | `smallint` | NIST 800-63 IAL, 1–3, ordered for cheap comparison |
| `self_registration` | `boolean` | Public realms only |
| `allowed_factors` / `required_factors` | `text[]` | Employees require TOTP; applicants may use email OTP |
| `session_ttl`, `access_token_ttl`, `refresh_token_ttl` | `interval` | Per population |
| `default_retention` | `interval` | Applicant data is **legally bounded**. NULL = no statutory limit. |
| `pii_encryption` | `boolean` | Enables crypto-shredding deletion |

> **Partners do not get their own tenant.** That would require cross-tenant
> grants, and every scope FK is composite on `tenant_id` precisely to make those
> impossible. Partner companies are scope nodes on a `partner` axis instead.

### `realm_categories`
Directory classification inside a realm — supplier vs contractor, applicant vs
customer — **runtime-extensible**: adding one is an INSERT (migration 0011).
`identities.category_id` carries a composite FK on `(category_id, realm_id)`,
so an identity **cannot** reference a category from another realm. Deliberate
limit: categories are directory metadata, never an authorization input — if two
categories need different policy, that is a second realm of the same kind; if
different access, that is roles.

### `identity_links`
The same human across realms — a supplier contact later hired, an applicant who
becomes an employee. Records `method` (`manual`, `email_proof`, `document`,
`hr_sync`) and evidence.

> Linking **never merges grants.** Inheriting a contractor's access on becoming
> an employee is exactly the accident this prevents.

### `consents`
Lawful basis for processing external users' personal data. **Append-only** — a
withdrawal is a new row, never an update, so the record of what was consented to
and when survives the withdrawal.

### `identities`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `uuid` PK | Stable, opaque. **Never reused, never an email.** |
| `tenant_id` | `uuid` FK | |
| `realm_id` | `uuid` FK | Population membership; composite FK on tenant |
| `username` | `text` | unique per **(tenant, realm)**, case-insensitive |
| `email` | `text` | unique per (tenant, realm) when present |
| `assurance_level` | `smallint` | NIST IAL 1–3. Gated against `permissions.min_assurance`. |
| `retention_until` | `timestamptz` | Statutory deadline, defaulted from the realm |
| `deletion_requested_at` | `timestamptz` | Right-to-erasure request |
| `anonymized_at` | `timestamptz` | Crypto-shredding done — **authorization denies from here** |
| `pii_key_id` | `uuid` | Per-identity PII key; deleting it shreds the data while rows survive for audit |
| `token_epoch` | `int` | **Global kill switch.** Bumping invalidates every outstanding token without touching a token row. |
| `status` | `text` | `active` \| `disabled` \| `locked` \| `pending` |
| `external_ref` | `text` | HR/ERP linkage |

Case-insensitive uniqueness uses expression indexes on `lower(username)` and
`lower(email)` rather than the `citext` extension.

**Uniqueness is per realm, not per tenant.** `alice` the employee and `alice` the
job applicant are different people. Verified: the same username exists in all
three realms simultaneously.

### `credentials`
**One row per authentication factor.** Password, device key (biometric), API key
and TOTP share this table — adding a factor never changes the schema.

| Column | Type | Notes |
| :--- | :--- | :--- |
| `kind` | `text` | `password` \| `device_key` \| `api_key` \| `totp` \| `recovery_code` \| `oidc_link` |
| `secret` | `text` | password: `$pbkdf2-sha256$i=600000$<salt>$<hash>` — algorithm and parameters **inline**, so the KDF can be upgraded by rehashing on next login. api_key: SHA-256 hex. device_key: Ed25519 public key, base64url. |
| `lookup_key` | `text` | Indexed prefix for API keys — auth is a single index probe, never a scan |
| `sign_counter` | `bigint` | WebAuthn/device-key replay defence; must increase monotonically |
| `params` | `jsonb` | |

Indexes enforce **at most one active password per identity** and unique
`lookup_key` among non-revoked rows.

---

## Sessions and tokens

### `sessions`
**The anchor of the whole design.** Sign-out revokes here; everything else
follows.

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `uuid` PK | becomes the `sid` claim |
| `amr` | `text[]` | how identity was proven: `{pwd}`, `{pwd,otp}` |
| `auth_time` | `timestamptz` | step-up recency |
| `active_scopes` | `jsonb` | active context per axis — **a map, never fixed columns** |
| `device_fp`, `ip`, `user_agent` | | |

`fillfactor = 80` — `last_seen_at` updates on every refresh, so HOT matters.

### `refresh_tokens` — partitioned by `expires_at`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `token_hash` | `bytea` | SHA-256. **The token itself is never stored.** |
| `family_id` | `uuid` | All descendants of one login |
| `successor_id` | `uuid` | Rotation chain |
| `status` | `text` | `active` \| `consumed` \| `revoked` |
| `bound_key` | `text` | Optional proof-of-possession public key |

Single-use. Presenting a `consumed` token means **theft** → revoke the entire
family. Retention is `DROP PARTITION`, not `DELETE`.

### `signing_keys`

`pending → active → retiring → retired`. Private key encrypted with a KMS-held
master key. At most one `active` key per purpose, enforced by a partial unique
index.

> `kid` is looked up **only** from a preloaded in-memory map. Never a database
> query on the verify path — attacker-controlled input must not drive I/O.

---

## Dynamic scope

The answer to *"add access dimensions without code changes."* Design in
[ADR-0003](adr/0003-scope-model.md).

### `scope_axes` — the table that makes scopes runtime-extensible

Adding "by product" is **one INSERT**. No DDL, no deploy.

| Column | Type | Notes |
| :--- | :--- | :--- |
| `code` | `text` PK | `^[a-z][a-z0-9_]{1,30}$` |
| `default_effect` | `text` | `unconstrained` — grants silent on this axis still pass (**safe to add**) · `deny` — grants must address it |
| `resolution` | `jsonb` | Where the target value comes from: `{"from":"token"}` or `{"from":"context","key":"product_id"}` |
| `ui_schema` | `jsonb` | Drives the generic admin UI. **The UI must render from this, never from a switch on axis code.** |
| `sort_order` | `int` | Deterministic explain output |

### `scope_node_types`
Node types **within** an axis. `office` and `department` both live on the org
axis. The root type is the one with empty `parent_types`.

### `scope_nodes`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `uuid` PK | |
| `tenant_id`, `axis_code`, `node_type`, `parent_id` | | |
| `slug`, `name` | `text` | unique among siblings |
| `external_ref` | `text` | **Idempotency key for ERP/CRM sync** |
| `is_axis_root` | `boolean` | Exactly one per (tenant, axis) |
| `attributes` | `jsonb` | **Display metadata only — never a decision input.** See below. |

Three composite FKs make illegal states unrepresentable:

```sql
UNIQUE (id, tenant_id, axis_code)                       -- FK target
FOREIGN KEY (node_type, axis_code)   REFERENCES scope_node_types(code, axis_code)
FOREIGN KEY (parent_id, tenant_id, axis_code)           -- same tenant AND axis
    REFERENCES scope_nodes(id, tenant_id, axis_code)
```

> **`attributes` is display metadata. It is never a decision input.** jsonb
> predicates are unindexable on the hot path, unauditable, and a typo'd key does
> not error — it silently matches nothing, and `false` in an authorization system
> is an outage nobody can diagnose. Anything that decides access becomes its own
> axis.

### `scope_closure`
Transitive reachability. **The hot-path table.**

```sql
ancestor_id   uuid      -- PRIMARY KEY (ancestor_id, descendant_id)
descendant_id uuid
depth         smallint  -- 0 = self
```

Deliberately narrow — `tenant_id` and `axis_code` omitted because the invariant
is enforced upstream and rows are written only by the maintenance functions.
**Measured: 71.9 bytes/row.**

`scope_closure_up (descendant_id, ancestor_id) INCLUDE (depth)` enables
index-only ancestor scans.

---

## Authorization

### `permissions`
Owned by applications, namespaced by application slug.

| Column | Type | Notes |
| :--- | :--- | :--- |
| `app_slug` | `text` | Copy of `applications.slug`, held correct by composite FK with `ON UPDATE CASCADE` — **cannot drift** |
| `key` | `text` | **`GENERATED ALWAYS AS (app_slug \|\| ':' \|\| resource \|\| ':' \|\| action) STORED`** |
| `risk` | `text` | `normal` \| `sensitive` \| `critical` |
| `min_assurance` | `smallint` | Minimum identity assurance. An IAL1 applicant cannot approve a payment **even with a grant**. |
| `requires_amr` | `text[]` | e.g. `{otp}` — step-up requirement |
| `max_auth_age` | `interval` | e.g. `5 minutes` |

`key` was originally trigger-maintained and came out **silently NULL** under
`session_replication_role='replica'`. A generated column is maintained by the
storage engine itself.

**`requires_amr` + `max_auth_age` on the permission** means one central
definition of "sensitive"; every application gets step-up for free.

### `roles`, `role_parents`, `role_permissions`, `role_permission_patterns`

Concrete grants and wildcard patterns are **separate tables** — different
lifecycles, and the combined version had an illegal expression primary key.

### `role_permissions_effective`
Fully flattened closure of the role graph × (concrete + expanded patterns).
Read path is a pure index join. `via_role_id` carries provenance, so "why does
Alice have this?" is a query.

**Wildcards are expanded at write time, never matched at read time** — evaluation
is pure set membership, auditing is a `SELECT`, and a newly registered permission
cannot silently widen an existing role.

### `grants` and `grant_scopes`

```sql
grants        (id, tenant_id, identity_id, role_id, valid_from, valid_until,
               revoked_at, granted_by, reason, self_scoped)

grant_scopes  (grant_id, tenant_id, axis_code, scope_node_id, inherit)
```

**`inherit` lives on `grant_scopes`, not `grants`** — inheritance is per-axis.
"Everything under Jakarta, but only Product Line A itself and not its SKUs" is a
legitimate requirement a grant-level flag cannot express.

```sql
FOREIGN KEY (scope_node_id, tenant_id, axis_code)
    REFERENCES scope_nodes (id, tenant_id, axis_code)
```

Makes cross-tenant and cross-axis grants impossible to insert.

`grants_identity_live` is partial + covering, so candidate lookup is an
index-only scan.

**`self_scoped`** means "only your own record" — the dominant external-user
shape, and not a scope question, since no tree node describes the row you own.
The caller passes the owner as the reserved target key `_owner`; reserved keys
begin with `_`, which `scope_axes.code`'s CHECK forbids, so an axis can never
collide. A `self_scoped` grant with no `_owner` supplied is **denied**.

### `role_grantable` and `roles.allowed_realm_kinds`

Delegated administration without escalation. Partners administer their own users,
so grant administration is no longer performed only by trusted IT staff.

- `allowed_realm_kinds` — which populations may hold a role
- `role_grantable` — which roles a role may confer

Enforced by constraint triggers, so a script bypassing the application layer
still cannot attach an employee-only role to a public self-registered account.

---

## Hosted pages

### `auth_pages`
The sign-in and sign-out pages a tenant's own people see. One row per page;
served at `/p/{tenant}/{kind}/{slug}`.

```sql
id             uuid PRIMARY KEY
tenant_id      uuid NOT NULL
kind           text NOT NULL   -- signin | signout
slug           text NOT NULL   -- URL segment; published, so immutable
name           text NOT NULL   -- admin-facing label
status         text NOT NULL   -- active | disabled
is_default     boolean         -- the tenant fallback for this kind
application_id uuid            -- optional binding
realm_id       uuid            -- optional binding (migration 0041)
config         jsonb NOT NULL  -- a token set, never markup
```

**A page binds to an application OR a realm, never both**
(`auth_pages_one_binding`). Resolution would otherwise have to pick one, and
whichever it picked would surprise somebody; refusing the row is cheaper than
documenting a precedence nobody remembers. Both bindings are UNIQUE per kind
(`auth_pages_one_per_app`, `auth_pages_one_per_realm`), and `realm_id` carries
a composite foreign key on `(realm_id, tenant_id)` so a page bound to another
tenant's realm is unrepresentable.

**Resolution, most specific first:**

```
explicit slug  ->  application  ->  realm  ->  tenant default
```

The realm sits between application and default deliberately: an application
that configured its own door keeps it, so adding realm pages changed nothing
that already resolved. The realm comes from `?realm=` (default `internal`),
which is also what the population picker posts.

`config` is validated by `internal/tenancy/domain/pagecfg` on write **and** on
read. A stored config that no longer parses — a downgrade, a hand-edited row —
logs an error and renders defaults rather than taking sign-in down.

### `signin_pages` — superseded
Migration 0024 replaced this with `auth_pages` and copied the rows across.
**Nothing renders from it.** The `GetSigninPage`/`PutSigninPage` RPCs still
read and write it and are deprecated; they take a shape `pagecfg` cannot
parse, so they cannot be forwarded to the real page. `anubis_deprecated_rpc_total`
counts callers, which is the evidence for when the table can go.

---

## Route policies

`route_policies` translates URL paths to permissions.
[ADR-0006](adr/0006-path-protection.md).

| Column | Notes |
| :--- | :--- |
| `priority` | **Explicit integer.** Implicit "most specific wins" is where people get surprised. |
| `effect` | `public` \| `require_auth` \| `require_permission` \| `deny` |
| `path_pattern`, `host_pattern`, `methods` | |
| `scope_bindings` | `jsonb` — how each axis resolves, e.g. `{"product":"path.id","org":"token"}` |

Everything unmatched is **denied**.

---

## Operations

### `audit_log` — partitioned by month

Append-only, hash-chained (`prev_hash` / `entry_hash`): an attacker with `UPDATE`
rights still cannot silently rewrite history.

`detail jsonb` stores the **decision inputs**, snapshotted — so past decisions
never need reconstructing from mutable catalog state.

Indexed with **BRIN** on `occurred_at`: append-only and physically time-ordered,
so BRIN is orders of magnitude smaller than the B-tree equivalent.

### `catalog_version`
One monotonic counter per tenant. The single invalidation signal, bumped by
**statement-level** triggers on every table feeding the hot-path snapshot, with
`pg_notify` for push. `fillfactor = 70` — updated constantly.

### `schema_migrations`
`version`, `checksum`, `applied_at`.

---

## Functions

| Function | Purpose |
| :--- | :--- |
| `authorize(identity, tenant, permission, targets jsonb) → boolean` | The decision. See [ADR-0004](adr/0004-authorization-semantics.md). |
| `scope_ensure_root(tenant, axis) → uuid` | Creates the axis root. Picks the node type with **no legal parents** (picking alphabetically produced org roots typed `department`). |
| `scope_add_node(...) → uuid` | Attaches a node, materialises closure edges. O(depth). |
| `scope_move_node(node, new_parent)` | Moves a subtree. **Cycle detection is one indexed probe.** Requires `SERIALIZABLE`. |
| `role_recompute_effective(role)` | Recomputes the flattened permission set. Recursive CTE with PG's `CYCLE` clause. |
| `bump_catalog_version(tenant)` | Invalidation + `pg_notify`. |
| `trg_grant_realm_guard()` | Rejects roles granted outside their allowed realm kinds. |
| `trg_self_scope_guard()` | Rejects axis constraints on self-scoped grants. |
| `ensure_month_partitions(table, col, ahead)` | Provisions partitions. Run on a schedule. |

---

## Enforced invariants

Nine security-critical invariants live in the schema, not in application code.
All are tested — [7/7](../bench/negative.sql) plus
[2/2](../bench/realms.sql) rejected.

| # | Illegal write | Rejected by |
| :--- | :--- | :--- |
| 1 | Grant referencing another tenant's scope node | `grant_scopes_scope_node_id_tenant_id_axis_code_fkey` |
| 2 | Grant claiming `axis=product` but pointing at an org node | same FK |
| 3 | Scope node parented across axes | `scope_nodes_parent_id_tenant_id_axis_code_fkey` |
| 4 | Node type belonging to a different axis | `scope_nodes_node_type_axis_code_fkey` |
| 5 | Permission `app_slug` forged | `permissions_application_id_app_slug_fkey` |
| 6 | Two axis roots for one (tenant, axis) | `scope_nodes_one_root` |
| 7 | Cycle in the scope tree | `scope_move_node` raise |
| 8 | Employee-only role granted to a public account | `grants_realm_guard` trigger |
| 9 | Axis constraints attached to a self-scoped grant | `grant_scopes_self_guard` trigger |

Guards 8 and 9 caught a real bug during development: the seed script was granting
employee roles to partner identities.

> The schema is what is still true after someone writes a bad migration script at
> 2am. Anything that would be a privilege-escalation or cross-tenant bug belongs
> here, not in a code review checklist.
