# API reference

> **Status: specification.** The database layer is built and validated; the HTTP
> and gRPC transports are not yet implemented. This document is the contract they
> will implement. See [roadmap.md](roadmap.md).

Base URL: `https://anubis.internal`

## Conventions

**Error envelope** — every non-2xx response:

```jsonc
{
  "error": "invalid_credentials",       // stable machine-readable code
  "message": "Invalid username or password",
  "request_id": "req_01HXY...",         // correlates to audit_log and traces
  "details": {}                         // optional, error-specific
}
```

**Authentication of callers**

| Caller | Mechanism |
| :--- | :--- |
| End user | `Authorization: Bearer <access_token>` |
| Application (service-to-service) | `Authorization: Bearer anb_live_<prefix>_<secret>` |
| Browser (SSO flows) | `__Host-anubis_sso` cookie |

**Rate limiting** — `429` with `Retry-After`. Limits apply on three axes: per IP,
per account, per tenant. Per-account is the one that stops credential stuffing.

---

## Authentication

### `POST /v1/auth/login`

```jsonc
// request
{ "tenant": "impack", "username": "alice", "password": "…",
  "client_id": "billing-web", "device_fp": "…" }
```

```jsonc
// 200 — authenticated
{ "access_token": "v4.public.eyJzdWIi…",
  "refresh_token": "anb_rt_…",
  "token_type": "Bearer", "expires_in": 600,
  "session_id": "ses_01HXY…" }
```

```jsonc
// 202 — second factor required
{ "mfa_required": true, "mfa_token": "anb.local.v1.…",
  "methods": ["totp", "device_key"], "expires_in": 60 }
```

> **Failure responses are uniform in message *and timing*.** The KDF runs even
> when the user does not exist (compared against a fixed dummy hash), because
> otherwise response timing is a user-enumeration oracle — invisible in
> functional testing, visible in a timing histogram.

### `POST /v1/auth/mfa/verify`

```jsonc
{ "mfa_token": "anb.local.v1.…", "code": "123456" }
```

### `POST /v1/auth/device/challenge` → `POST /v1/auth/device/verify`

Biometric login. **Biometric data never reaches the server.**

```jsonc
// challenge
{ "tenant": "impack", "device_id": "dev_…" }
→ { "nonce": "…", "expires_in": 60 }

// verify — device signed the nonce after a local biometric unlock
{ "nonce": "…", "key_id": "cred_…", "signature": "base64url…" }
```

The device generates a keypair inside Secure Enclave / Android Keystore, gated on
biometric unlock. Anubis stores only the public key and does exactly one
`ed25519.Verify`. Nonces are single-use with **atomic** consumption (Redis
`GETDEL` or a Lua script) — `GET` then `DEL` leaves a replay window.

### `POST /v1/auth/token/refresh`

```jsonc
{ "refresh_token": "anb_rt_…" }
```

Returns a **rotated** pair. Presenting an already-consumed token revokes the
entire family and the session:

```jsonc
// 401
{ "error": "refresh_token_reuse_detected",
  "message": "Token family revoked. Re-authentication required." }
```

**This response must page a human.** It means a token was stolen.

### Sign-out

| Endpoint | Effect |
| :--- | :--- |
| `POST /v1/auth/logout` | Revoke the current session (this device) |
| `POST /v1/auth/logout/all` | Revoke every session, bump `token_epoch` |
| `POST /v1/auth/logout/session/{id}` | Revoke one named session |

Global logout triggers **back-channel logout**: Anubis POSTs a signed logout
token to each application's `backchannel_logout_uri`. Without it, an application
with its own session cookie keeps the user logged in after they signed out.

---

## Tokens

### `POST /v1/auth/introspect` — service auth only

```jsonc
{ "token": "v4.public.…" }
→ { "active": true, "sub": "usr_…", "sid": "ses_…", "tid": "tnt_impack",
    "roles": ["finance.approver"], "scopes": {"org":"…"},
    "amr": ["pwd","otp"], "exp": 1735689600 }
```

For applications needing instant revocation. Most should verify offline instead —
introspection puts Anubis in the hot path.

### `POST /v1/auth/revoke`

### `GET /.well-known/anubis-keys.json`

Public keys with `kid`, served with `Cache-Control`. Clients refetch on unknown
`kid`.

### `GET /.well-known/openid-configuration`

Discovery document. OIDC-shaped by choice, so standard client libraries remain an
option.

---

## Authorization

### `POST /v1/authorize`

```jsonc
{ "subject": "usr_01HXY…",
  "permission": "billing:invoice:approve",
  "scopes": { "org": "01a027ff-…", "product": "01a027fa-…",
              "customer": "01a027fb-…" },
  "context": { "amount": 50000000 } }
```

```jsonc
// allowed
{ "allow": true }

// denied — the failing axis is always named
{ "allow": false, "reason": "scope_mismatch",
  "failing_axis": "customer",
  "message": "no grant at or above customer node 01a027fb-…" }

// step-up required — machine-readable, so the app does not guess
{ "allow": false, "reason": "step_up_required",
  "required_amr": ["otp"], "max_auth_age": "2m",
  "current_amr": ["pwd"], "auth_age": "41m" }
```

> **Supply every axis the grant might constrain.** An omitted axis is
> **denied**, not ignored — see
> [ADR-0004](adr/0004-authorization-semantics.md#fail-closed-is-the-whole-design).

### `POST /v1/authorize/explain`

Not optional once more than two axes exist. Returns which grant matched, which
role conferred the permission, and — on denial — exactly which axis failed and
why.

### `POST /v1/gate/check` — forward-auth

For nginx `auth_request`, Traefik `forwardAuth`, Envoy `ext_authz`.

| Header in | |
| :--- | :--- |
| `X-Original-URI` | Full request URI |
| `X-Original-Method` | |
| `X-Original-Host` | |

| Status | Meaning | Response headers |
| :--- | :--- | :--- |
| `204` | Allow | `X-Anubis-Subject`, `X-Anubis-Scope`, `X-Anubis-Session` |
| `401` | Needs login | `Location` for the redirect flow |
| `403` | Denied | |

Served entirely from the in-memory snapshot. **Target p99 < 1 ms, no database on
the path.** Fail-static if Postgres is down; fail-closed only if the snapshot
exceeds its maximum age.

---

## Browser SSO

### `GET /v1/authorize` — authorization code flow with PKCE

```
?response_type=code&client_id=…&redirect_uri=…&state=…
&code_challenge=…&code_challenge_method=S256&scope=openid+profile
```

> `redirect_uri` is matched **exactly** against the application's allowlist. No
> wildcards, no prefix matching. Open redirect here is full account takeover.

### `POST /v1/token`

Exchanges `code` + `code_verifier` for tokens.

The `__Host-anubis_sso` cookie on the Anubis domain **is** the "single" in single
sign-on: a second application's redirect finds the existing session and bounces
straight back with a code, no prompt.

---

## Identity and scope administration

| Endpoint | Purpose |
| :--- | :--- |
| `GET /v1/me` | Profile, tenant, roles, effective permissions |
| `GET /v1/sessions` · `DELETE /v1/sessions/{id}` | User-visible device list |
| `POST /v1/auth/scope/switch` | Re-issue an access token for a different active scope **without re-authentication** — mirrors AWS `AssumeRole` |
| `POST /v1/apikeys` · `DELETE /v1/apikeys/{id}` | Format `anb_live_<prefix>_<secret>`; indexed prefix, hashed secret |

### Realms and external users

```http
POST /v1/admin/realms
{ "code": "partner", "kind": "partner", "display_name": "Suppliers",
  "min_assurance": 2, "self_registration": false,
  "allowed_factors": ["password","totp"], "required_factors": ["password","totp"],
  "session_ttl": "8 hours" }
```

| Endpoint | Purpose |
| :--- | :--- |
| `POST /v1/register` | Self-registration — **public realms only**, gated on `realms.self_registration` |
| `POST /v1/identities/{id}/link` | Link the same human across realms. Records method and evidence; **never merges grants**. |
| `POST /v1/consents` · `DELETE /v1/consents/{id}` | Lawful-basis capture. Withdrawal appends a row, never updates one. |
| `POST /v1/identities/{id}/erasure` | Right-to-erasure request → sets `deletion_requested_at` |
| `POST /v1/admin/identities/{id}/disable` | **Immediate, complete deprovisioning.** `authorize()` gates on identity state, so no grant needs touching. |

Delegated administration uses ordinary permissions — `anubis:identity:create`,
`anubis:grant:create` — scoped to a `partner_org` node. **Anubis is its own
relying party.** Escalation is blocked in the schema by
`roles.allowed_realm_kinds` and `role_grantable`, not by API validation.

Self-scoped access passes the record owner as the reserved key `_owner`:

```jsonc
POST /v1/authorize
{ "subject": "usr_applicant…", "permission": "ats:application:read_own",
  "scopes": { "_owner": "usr_applicant…" } }
```

A `self_scoped` grant with no `_owner` supplied is **denied**.

### Scope axes — the dynamic dimension API

```http
POST /v1/admin/scope-axes
{ "code": "product", "display_name": "Product Line",
  "default_effect": "unconstrained",
  "resolution": { "from": "context", "key": "product_id" },
  "ui_schema": { "picker": "tree", "searchable": true, "icon": "box" } }

POST /v1/admin/scope-node-types
{ "code": "product_line", "axis_code": "product", "parent_types": [] }

PUT  /v1/admin/scopes/product/nodes        # bulk upsert, keyed on external_ref
```

Three calls. **No deploy, no migration, no restart.** Verified: adding a
`cost_center` axis with 511 nodes changed **0 of 2,000** existing decisions.

```http
POST /v1/admin/scope-axes/{code}/strict-dry-run
```

Replays recent decisions against `default_effect='deny'` and reports what would
break. **Run this before flipping an axis to strict.** On the seed data the flip
took allows from 800 → 0 — better learned from a report than an outage.

### Synchronising a structure from where it actually lives

Structures usually belong to another system: the ERP owns cost centres, the CRM
owns customers, HR owns the org chart. Register **one source of truth per
axis** and Anubis pulls from it — over that source's **own connection**, which
is a different database, a different credential, possibly a different engine
than Anubis's own.

| `kind` | Config | Used for |
| :--- | :--- | :--- |
| `http` | `{"url":…, "auth_header":…}` | A JSON API returning `[{ref, parent_ref, name, node_type?}]` |
| `db_query` | `{"dsn":…, "query":…}` | **Another database**; the operator's own SQL, aliased to `ref`/`parent_ref`/`name` |
| `db_table` | `{"dsn":…, "table":…, "columns":{"ref":…,"parent_ref":…,"name":…}}` | **A table in another database**, mapped column-by-column |

```jsonc
// register once
POST /v1/admin/scope-sync-sources
{ "axis": "cost_center", "kind": "db_table",
  "config": { "dsn": "postgres://reader:…@erp-db:5432/erp",
              "table": "cost_centers",
              "columns": {"ref":"id","parent_ref":"parent_id","name":"label"},
              "default_node_type": "cost_center" } }

// then, on a schedule or on demand — no rows in the body means PULL
POST /v1/admin/scope-sync-sources/{id}/run     { "dry": true }
→ { "added": 5, "renamed": 0, "moved": 0, "archived": 0, "unchanged": 0, "errors": [] }
```

Rows may also be **pushed** in the request body when the source cannot be
reached from Anubis (an air-gapped ERP, a nightly export).

Rules that make this safe to run unattended:

- **Keyed on `external_ref`** — same ref, same node, forever. Renames and moves
  follow; ids never churn.
- **Archive, never delete.** A row that vanishes from the source archives its
  node: existing grants keep deciding, pickers stop offering it.
- **Manual nodes are untouched.** Only ref-carrying nodes are sync's to
  archive; a node created by hand in the console is never removed by a feed.
  *(Verified against ~31,000 hand-seeded nodes: zero archived.)*
- **An unreachable source is an error, not an empty feed.** Returning "no rows"
  would archive the entire axis; the run fails loudly instead.
- **Parents-first is guaranteed by Anubis, not assumed of the source.** No SQL
  `ORDER BY` can express a topological order, so feeds are sorted before they
  are applied.
- **`dry: true` reports the same numbers with zero writes** — including rows
  whose parent would only be created by that same run.
- **Rotating a credential is `UpdateSyncSource`**, not delete-and-recreate; the
  source keeps its history.
- Level rules still bite: inserts and moves go through `scope_add_node` /
  `scope_move_node`, so an illegal placement is captured as that row's error
  without aborting the run.

### Manifests

```http
PUT /v1/admin/applications/{slug}/manifest
```

Registers permissions, roles and route policies. Anubis validates, diffs against
the current catalog, and applies. Removed permissions are marked `deprecated_at`,
never deleted — never orphan a live grant. Shadowed route rules are rejected.

---

## Health

| Endpoint | Checks |
| :--- | :--- |
| `GET /healthz` | Process alive |
| `GET /readyz` | Database reachable, snapshot within max age, active signing key present |

---

## gRPC

`introspect`, `authorize` and `gate check` are also exposed over gRPC for
service-to-service traffic — lower latency, binary framing, and streaming for
revocation events. Same use cases behind both transports; the transport layer
contains no business logic.
