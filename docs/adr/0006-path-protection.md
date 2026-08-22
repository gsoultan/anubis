# ADR-0006 — Securing website paths

**Status:** accepted · **Date:** 2026-08-22

## Context

Applications need to protect URL paths (`/admin/**`, `POST /invoices/{id}/approve`)
— including applications whose code we cannot modify.

## Routes map *to* permissions; they are not permissions

The shortcut is to make the path the resource:
`permission(resource='/admin/users', action='GET')`. **Rejected.** It couples
ACLs to URL structure, so a routing refactor silently breaks authorization, and
"can approve invoices" becomes inexpressible independently of how it is reached
(HTTP, gRPC, batch job, admin CLI).

The permission model stays transport-agnostic. A thin `route_policies` layer
translates.

```json
"routes": [
  {"priority": 10,  "path": "/healthz",   "effect": "public"},
  {"priority": 20,  "path": "/static/**", "effect": "public"},
  {"priority": 100, "path": "/admin/**",  "effect": "require_permission",
                    "permission": "billing:admin:access"},
  {"priority": 200, "methods": ["POST"], "path": "/invoices/{id}/approve",
                    "effect": "require_permission",
                    "permission": "billing:invoice:approve",
                    "scope_bindings": {"product": "path.id", "org": "token"}}
]
```

**Everything unmatched is denied.** Default-deny with an explicit public list is
the only safe default; the inverse ships every new route unprotected until
someone remembers.

**Explicit integer priority, not "most specific wins."** Implicit specificity is
where people get surprised, and surprises in authorization are outages. A
shadow-detection check at manifest registration rejects rules that can never
match.

## Three enforcement modes

| Mode | Mechanism | Fits |
| :--- | :--- | :--- |
| **1. SDK middleware** | `pkg/anubis` in-process: verify offline, match routes from a cached snapshot. **Zero network calls.** | Our Go services |
| **2. Forward-auth** | nginx `auth_request`, Traefik `forwardAuth`, Envoy `ext_authz` → `POST /v1/gate/check`. **No application code changes.** | Any language, legacy apps, third-party software |
| **3. Gate sidecar** | Small reverse proxy holding the policy snapshot locally | No gateway available, or fail-static isolation wanted |

```nginx
location /admin/ {
    auth_request /_anubis;
    auth_request_set $user $upstream_http_x_anubis_subject;
    proxy_set_header X-User $user;
    proxy_pass http://backend;
}

location = /_anubis {
    internal;
    proxy_pass              http://anubis/v1/gate/check;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URI    $request_uri;
    proxy_set_header X-Original-Method $request_method;
}
```

`204` allow · `401` needs login · `403` denied. The application never learns
Anubis exists.

## Resolving the hot-path contradiction

[Principle 1](../architecture.md#design-principles) says Anubis is never
synchronously in the request path. Forward-auth puts it exactly there, on every
request to every protected path. This is the main engineering risk in the
feature.

`/v1/gate/check` is therefore a **different endpoint** from `/v1/authorize`:

- **No database on the path.** Each replica holds an in-memory snapshot — route
  table, `role_permissions_effective`, scope closure, active public keys —
  refreshed on `catalog_version` change.
- **Token verification is offline** — `ed25519.Verify`, ~50 µs, no I/O.
- **The decision is set membership** against pre-flattened permissions. This is
  precisely why wildcards are expanded at write time; runtime pattern matching
  here would be fatal.
- **Budget: p99 < 1 ms, zero allocations on the happy path.** Measured and
  alerted.
- **Fail-static, not fail-closed.** If Postgres is down the snapshot still
  answers. A stale-but-correct decision beats a company-wide outage. Fail closed
  only when the snapshot exceeds its maximum age.

Mode 3 is the stronger version: the snapshot lives beside the application, so
even Anubis being completely down does not break path enforcement for
already-issued tokens.

## Path normalisation is the security-critical part

This is where real CVEs live — nginx, Envoy and several API gateways have all
shipped path-confusion bypasses. If `/admin/**` is protected, none of these may
slip through:

```
/admin/../admin/users        /admin%2Fusers          /%61dmin/users
//admin//users               /admin/users/           /ADMIN/users
/admin/users;jsessionid=x    /admin/users%00.jpg
```

**Normalise before matching, then reject anything still ambiguous.**
Percent-decode **once** (double-decode is its own vulnerability class), reject
any remaining `%` decoding to `/` or `\`, collapse duplicate slashes, resolve
`.` and `..`, strip trailing slash and path parameters, reject null bytes and
control characters.

> **The gate and the application must normalise identically.** If nginx
> normalises one way and Go's `ServeMux` another, the gap between them is the
> bypass.

Pin this with a shared corpus of adversarial paths run against both, plus a Go
fuzz target.

## Consequence: browser flows become mandatory

A browser sends no `Authorization` header on a plain navigation, so protecting a
path for a browser requires the **authorization code flow with PKCE**:

```
GET /admin/reports            → gate: no session cookie
  ← 302 to anubis/authorize?client_id=…&redirect_uri=…&state=…&code_challenge=…
       user authenticates
  ← 302 back with ?code=…
       app exchanges code → tokens, sets its own session cookie
  → GET /admin/reports        → gate: valid session, permission check, 204
```

This adds to Phase 1:

- SSO session cookie on the Anubis domain — `__Host-anubis_sso`, `HttpOnly`,
  `Secure`, `SameSite=Lax`
- `state` parameter and PKCE `code_verifier`/`code_challenge`
- **Strict `redirect_uri` allowlisting — exact match, no wildcards, no prefix
  matching.** Open redirect here is full account takeover.
- A hosted login page (`html/template`)
- Back-channel logout

The payoff: that cookie **is** the "single" in single sign-on. Log into billing,
navigate to HR, and the redirect finds the existing SSO cookie and bounces
straight back with a code — no second prompt. Without it you do not have SSO, you
have a shared token service.
