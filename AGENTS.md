# Anubis — engineering conventions & developer profile roster

## Architecture (mandatory)

**Bounded contexts** (ADR-0010): `identity`, `auth`, `authz`, `scope`,
`tenancy`, `audit`, `gate`, plus `shared/` (kernel) and `platform/`
(technical). Inside each context the layering is
**domain → port → app → service → endpoint → adapter**, programmed by
interface at every seam.

Hard rules, all CI-enforced (`scripts/check/*.sh`):

- **≤ 10 Go files per folder.** Outgrowing it means a missing concept —
  split by aggregate (`authz/domain/grant`), not by adding files.
- **≤ 15 methods per interface**; compose wider capabilities by embedding.
- **One interface per file, one struct per file.** Methods may live in
  topical files; the type is defined exactly once.
- **Folders name the layer, package clauses are context-prefixed and
  unique** (`internal/identity/domain` → `package identitydomain`). No
  aliases at import sites, no shadowing of common locals.
- **`*/domain` and `shared/*` import stdlib only.**
- **No context imports another context's `adapter/…`** — cross-context
  traffic goes through ports and domain types.
- **SQL only in a context's `adapter/postgres/rquery/` and `migrations/`**
  (ADR-0009 §8), one generated `rgen/` package per context. The schema of
  record is `internal/platform/schema`; `cmd/stormddl` writes migrations
  from it. sqlc and `db/queries/` are gone.
- go-kit middleware composes ONLY at the endpoint layer; transports carry no
  business logic.

| Profile | Owns | Vetoes | Proof |
| :--- | :--- | :--- | :--- |
| **sec** | crypto/, pkg/anubis, login/token/OIDC flows, guards | a fast path that skips a check; any secret compared with `==`; `math/rand` anywhere; `kid`/attacker input driving I/O; a redirect_uri match that is not exact | timing histogram for enumeration paths; tamper/fuzz tests; bench/negative.sql green |
| **perf** | authorize path, snapshot, pgx pool, hot queries | a check that allocates per request; a query added to the hot path without EXPLAIN (ANALYZE, BUFFERS); benchmarking the subquery instead of the decision | end-to-end benchmark vs budget (authorize p95 < 2 ms, verify ~50 µs, gate p99 < 1 ms); benchstat delta |
| **dba** | migrations/, internal/platform/schema, each context's rquery/, bench/ | SQL strings in Go; a destructive migration step; editing a migration that has shipped (every install logs checksum drift on every boot, and `baseline` cannot clear it); row-level triggers on bulk-write tables; a cache that ignores tenant | `gen-drift.sh` and `stormddl -check` clean; bench/rebuild.sh green; forward-only numbered migration |
| **arch** | layer boundaries, ports, endpoint layer, proto surface | domain importing anything non-stdlib; proto or pgx types crossing into usecase; business logic in a transport | scripts/check/import-boundary.sh; buf breaking |
| **api** | proto/, connectapi, httpapi, error mapping | an error without a stable machine code; a breaking proto change without a version; an endpoint bypassing the endpoint middleware chain | buf lint + breaking; e2e suite |
| **test** | test suites, corpus files, CI gates | a bug fix without a failing-first test; a map keyed by attacker input without bound + eviction; skipped suites reported as done | `go test -race -shuffle=on` green; coverage: domain 100%, usecase ≥ 90% |
| **console** | ui/ — the operator console: DataTable, Page, design tokens in src/styles/index.css | a raw colour below the token blocks; a filter that narrows a page the server already paged, so the count lies; a screen that asks for every row of a table holding 50k; a column that prints the same sentence on every row | `bun run build` clean; `bun run smoke` green (every screen by URL at both widths, in CI after e2e); the screen opened against the seeded dev stack in both colour schemes |

Standing truths (all profiles): a fast path that skips a check is a
vulnerability; a check that allocates per request is a regression; a cache
that ignores who asked is a data leak; any map keyed by attacker-supplied
input needs a bound and an eviction; every bug fix ships a test that fails
before and passes after, with the root cause named in one sentence.
