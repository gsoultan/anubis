# ADR-0010 — Bounded contexts, and folders that stay small

**Status:** accepted · **Date:** 2026-08-23

## Context

The application layer grew as horizontal layers: one `repository` package
(76 files), one `usecase` package (77), one `postgres` adapter (35). Every
layer knew every subject. Adding a field to a grant meant opening the same
folder as adding a field to a session, and one `Store` type carried methods
for identities, tokens, scopes, audit and the gate at once.

That shape has two failure modes. Navigationally, a 77-file folder has no
grain — nothing tells you where a change belongs. Structurally, a single
`Store` implementing every port means no compiler check stops the token code
from reaching into audit internals.

## Decision

**Carve by domain, not by layer.** Seven bounded contexts, each owning its
own domain types, ports, application services and adapters:

| Context | Question it answers |
| :--- | :--- |
| `identity` | Who exists? (identities, credentials, realms, consents) |
| `auth` | How do you prove it? (sessions, tokens, sign-in, MFA, devices) |
| `authz` | What may you do? (roles, permissions, grants, memberships, decisions) |
| `scope` | Where may you do it? (axes, node trees, external sync) |
| `tenancy` | Whose installation is this? (tenants, applications, routes) |
| `audit` | What happened? (hash-chained log) |
| `gate` | May this request pass? (snapshot read model, forward auth) |

Supporting, deliberately *not* contexts: `shared/` (errors, principal, clock,
validation, JSON helpers) and `platform/` (config, crypto, database plumbing,
migration runner, go-kit middleware, rate limiting).

Inside a context the layering the project already used still holds:

```
domain/ ← port/ ← app/ ← service/ ← endpoint/ ← adapter/{postgres,rpc,http}
```

**Every folder holds at most 10 Go files** (`scripts/check/folder-size.sh`).
The limit is a forcing function: when a package outgrows it, the answer is
almost always a concept that wants its own package — which is how
`authz/domain/grant`, `authz/domain/membership` and `auth/app/{signin,mfa,
device,session,token}` came to exist.

**Package clause ≠ folder name, on purpose.** Folders name the *layer*
(`domain`, `port`, `app`); package clauses are context-prefixed and unique
(`identitydomain`, `authport`, `scopeapp`). Importers therefore never need
aliases, and no package name collides with a common local variable — a
package literally named `identity` would shadow `identity := ...` in the very
code that needs it most.

**One generated package per context.** `sqlc.yaml` emits
`internal/<context>/adapter/postgres/gen` from `db/queries/<context>`, so no
generated package holds another context's SQL, and each stays small.

**One repository type per context**, over a shared `platform/database` that
owns the pool, the ambient-transaction convention and the error mapping. The
god-`Store` is gone: `identitypg.Repository` cannot call an audit query
because it does not have one.

## Enforcement

| Check | Rule |
| :--- | :--- |
| `folder-size.sh` | ≤ 10 Go files per folder (generated code exempt) |
| `import-boundary.sh` | `*/domain` and `shared/*` import stdlib only |
| `context-boundary.sh` | no context imports another context's `adapter/…` |
| `no-sql-in-go.sh` | SQL only in `db/queries/<context>` and `migrations/` |

## Consequences

**Positive** — a change has an obvious home; contexts are independently
readable and independently testable; the compiler now enforces boundaries
that were previously conventions; new subdomains cost a folder, not a
rewrite.

**Negative** — more packages (≈50 vs ≈15), and cross-context flows (login
touches identity, auth and tenancy) now import three packages instead of
one. Accepted: those imports are the dependency graph made visible, which is
the point. A handful of helpers had to be exported to cross the new lines
(`authapp.SessionEstablisher`, `apiconnect.Err`, `apihttp.WriteError`); each
was already shared in practice.

**Unchanged** — every behavioural contract. The refactor kept the full unit,
fuzz and live e2e suites green throughout; no migration, proto or query text
changed except the per-context split of `db/queries`.
