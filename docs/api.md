# API reference

> **Status: implemented.** The transports (Connect RPC + the browser-facing
> HTTP flows) serve this contract on `:7448`. For the integration walkthrough —
> how a relying party actually wires these endpoints together — see
> [integration.md](integration.md).

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
`ed25519.Verify`. Nonces are single-use with **atomic** consumption: `DELETE … RETURNING` on
`one_time_tokens`, which has GETDEL semantics — a second presentation finds
no row. `SELECT` then `DELETE` would leave a replay window. (There is no
Redis in this system; an earlier draft of this page assumed one.)

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
| `GET`/`POST` `/v1/admin/identities/{id}/attributes` | The **encrypted** part of an identity (ADR-0013) — see below |

#### Attributes — the one encrypted column

`identities.attributes` is free-form, tenant-defined, and the only column
Anubis encrypts at rest. Everything else on an identity is a lookup or join
key; this is the one whose contents are arbitrary enough to be a home address
or a case note.

```http
POST /v1/admin/identities/{id}/attributes
{ "attributes": { "employee_id": "E-4471", "cost_centre": "CC-12" } }
```

- **Replace, not merge.** Sending an empty map clears the attributes. There is
  no partial update, because a partial update cannot express "delete this
  field" — and a field that outlives an erasure request is the failure this
  column exists to prevent.
- **Names are sealed too.** The whole map is one ciphertext, so `diagnosis` is
  no more visible in a database dump than its value.
- **Bounded**: 64 attributes, 128-byte keys, 8 KiB total.
- **`erased: true`** on read means the identity's key was shredded. The data
  is unrecoverable — a completed erasure reported as a fact, not an error.
  Writing attributes to an erased identity is refused (`409`) rather than
  silently minting a new key and undoing the erasure.

The key is per identity and stored sealed under the master key, so neither a
dump of `identities` nor of `pii_keys` is any use on its own.

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

### Sign-in and sign-out pages

A tenant serves **many** of each, because one login screen rarely fits every
audience: staff, partners and customers differ in branding, wording and which
options should even appear. Each page has its own URL.

```http
GET /p/{tenant}/{kind}/{slug}        # kind = signin | signout
```

```jsonc
POST /v1/admin/auth-pages
{ "kind": "signin", "slug": "partners", "name": "Partner portal",
  "application_id": "01a0…",          // optional: binds the page to an app
  "config": {
    "brand":  { "title": "Impack Partners", "logo_url": "https://cdn…/logo.svg",
                "primary_color": "#0f766e", "background_color": "#f8fafc",
                "text_color": "#111827", "corner_radius": "lg", "font": "system" },
    "layout": "split",                 // centered | split | minimal
    "copy":   { "heading": "Partner sign-in",
                "subheading": "Use the account your account manager issued.",
                "username_label": "Company email", "submit_label": "Continue" },
    "links":  [ { "label": "Contact support", "url": "https://help…" } ],
    "features": { "show_realm_picker": false, "show_registration": false,
                  "show_forgot_password": true, "remember_me": false }
  } }
```

Sign-out pages use the same brand/layout/copy plus:

```jsonc
{ "copy": { "confirm_heading": "Sign out?", "confirm_body": "…",
            "heading": "You have been signed out", "return_label": "Back to the app" },
  "behavior": { "confirm": true, "auto_redirect_seconds": 0,
                "default_return_url": "https://app.example.com/" } }
```

| Endpoint | Purpose |
| :--- | :--- |
| `ListAuthPages` · `GetAuthPage` | Inventory, with the URL each page is served at |
| `CreateAuthPage` · `UpdateAuthPage` · `DeleteAuthPage` | Manage pages. `kind` and `slug` are immutable — the URL is published |
| `SetDefaultAuthPage` | Promote a page; exactly one default per kind, enforced by a partial unique index |
| `PreviewAuthPage` | Validate a draft without saving, so the builder shows the same errors the save would |

**Which page renders.** Most specific first: `?page=<slug>` → the application's
own page → the tenant default. A missing or disabled page falls through rather
than failing the flow: losing branding must never cost a user the ability to
sign in.

> **Configuration, never markup.** Every field is an enum, a bounded string, a
> validated `#rrggbb` colour or an `http(s)` URL. There is no `custom_html` or
> `custom_css` knob, and there will not be one: these pages are served on
> Anubis's own origin on the screen where users type their password, so a
> markup field would hand every tenant admin stored XSS against their own
> users. Text is escaped and rendered; `<script>` in a heading shows up as
> text. Pages ship `X-Frame-Options: DENY` and a `default-src 'none'` CSP.

### RP-initiated sign-out

```http
GET  /v1/logout?tenant=…&post_logout_redirect_uri=…&page=…    # asks first
POST /v1/logout                                               # confirmed
```

`GET` renders the tenant's sign-out page and **asks**. That confirmation is
not politeness: a bare GET that ends sessions is reachable from any page on
the internet with an `<img>` tag. The POST carries a CSRF token bound to a
cookie, and the token rotates on every render — a token that survived a
rejected attempt would be replayable.

> `post_logout_redirect_uri` is matched **exactly** against the application's
> `post_logout_redirect_uris`, a separate allowlist from `redirect_uris`. A
> login callback is not a place to land after signing out, and an open
> redirect here is a phishing primitive: "you have been signed out, sign in
> again" is far more convincing when the link really did come from the
> identity provider. An unregistered address is refused and the user is told,
> rather than silently sent somewhere unexpected.

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
