# ADR-0008 — Connect RPC + go-kit as the transport and application framework

**Status:** accepted · **Date:** 2026-08-22

## Context

The application layer needs a transport the console (browser, TypeScript) and
services (Go, anything gRPC) can both consume, and an application-layer
structure that keeps cross-cutting concerns (rate limiting, logging,
instrumentation) out of business logic. ADR-0002 tier 3 already accepts
"gRPC/protobuf" as an infrastructure exception; this ADR names the concrete
libraries and where the line sits.

## Decision

**Transport: Connect RPC** (`connectrpc.com/connect`, currently v1.20.0 in Go).
One `net/http` handler serves three protocols — Connect (what the console
speaks via `@connectrpc/connect` **v2** and `@connectrpc/connect-web`), gRPC,
and gRPC-Web. "Connect v2" refers to the protocol family and the TypeScript
runtime major; the Go module is still on its v1 import path and implements the
same wire protocols. This collapses the roadmap's Phase 8 "gRPC transport"
into day one: the same handlers already speak gRPC.

**Framework: go-kit** (`github.com/go-kit/kit`). The service layer
(`internal/usecase`) stays plain Go; every operation is wrapped in a go-kit
`endpoint.Endpoint`, and cross-cutting concerns compose as endpoint
middleware in one place regardless of transport:

```
transport (connect / stdlib http)
   → endpoint.Chain(recover, requestID, logging, ratelimit, metrics)
      → usecase (plain Go, no framework imports)
         → ports → adapters
```

`internal/domain` and `internal/usecase` import neither connect, go-kit, nor
pgx — the framework stops at the endpoint layer. CI enforces the domain rule.

### The API surface splits in two

| Surface | Endpoints | Why |
| :--- | :--- | :--- |
| Connect (`/anubis.v1.*Service/*`) | login, MFA, device, refresh, logout×3, introspect, revoke, authorize, explain, scope switch, sessions, me, all admin | RPC-shaped; consumed via generated clients |
| stdlib `net/http` | `GET /v1/authorize`, `POST /v1/token` (OIDC PKCE — wire format fixed by spec), `/.well-known/*`, `/v1/gate/check` (nginx `auth_request` headers, p99 < 1 ms), `/healthz`, `/readyz`, hosted login page | wire shape dictated by OIDC, reverse proxies, k8s |

Both mount on one `http.Server` on `:7448` (h2c in dev so gRPC works over
cleartext). Neither surface contains business logic.

### Error model

One mapping table (`internal/adapter/connectapi/errors.go`). Domain errors
carry a stable machine code; on the stdlib surface they render as the api.md
envelope (`error`, `message`, `request_id`, `details`), on Connect as
`connect.NewError` with an `anubis.v1.ErrorInfo` detail carrying the same code
and `request_id`. The console sees identical codes on both surfaces.

## Dependencies added

| Module | Tier under ADR-0002 |
| :--- | :--- |
| `connectrpc.com/connect` | infrastructure (RPC framing) — accepted |
| `google.golang.org/protobuf` | infrastructure (wire format) — accepted |
| `github.com/go-kit/kit` | infrastructure (application framing) — accepted |

None sits on a cryptographic path. `pkg/anubis` — the SDK consumers embed —
remains a **zero-dependency** nested module; it must never import any of the
above.

## Consequences

**Positive** — one handler, three protocols; browser consumption without a
gateway; middleware composition in exactly one layer; gRPC for free.

**Negative** — go-kit's `any`-typed endpoints erase types between transport
and service; the transport layer re-asserts them. Accepted: the layer is thin,
mechanical, and tested through the e2e suite.
