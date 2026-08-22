# Anubis — engineering conventions & developer profile roster

## Architecture (mandatory)

Layering: **repositories → usecases → services → endpoints → transports**.
Programming by interface at every seam. No interface wider than **15
methods** — compose wider capabilities by embedding (see
`internal/repository/catalog_repository.go`). **One interface per file, one
struct per file** (methods may live in topical files; the type definition
appears exactly once). Security-critical flows are single-method usecases
(`Execute`); admin planes use grouped ≤15-method usecase interfaces.
go-kit middleware (recover, request-id, logging, metrics, rate-limit)
composes ONLY at the endpoint layer. Transports (Connect RPC + stdlib HTTP)
contain no business logic. SQL exists only in `db/queries/*.sql` (sqlc) and
`migrations/`.

Non-trivial changes are worked as a pair: adopt the **Driver** profile that
owns the code you touch, then re-read the diff as the **Challenger** whose
budget it most likely breaks. Name both in the task summary
(`Driver: sec · Challenger: perf`).

| Profile | Owns | Vetoes | Proof |
| :--- | :--- | :--- | :--- |
| **sec** | crypto/, pkg/anubis, login/token/OIDC flows, guards | a fast path that skips a check; any secret compared with `==`; `math/rand` anywhere; `kid`/attacker input driving I/O; a redirect_uri match that is not exact | timing histogram for enumeration paths; tamper/fuzz tests; bench/negative.sql green |
| **perf** | authorize path, snapshot, pgx pool, hot queries | a check that allocates per request; a query added to the hot path without EXPLAIN (ANALYZE, BUFFERS); benchmarking the subquery instead of the decision | end-to-end benchmark vs budget (authorize p95 < 2 ms, verify ~50 µs, gate p99 < 1 ms); benchstat delta |
| **dba** | migrations/, db/queries/, bench/ | SQL strings in Go; a destructive migration step; row-level triggers on bulk-write tables; a cache that ignores tenant | sqlc generate clean; bench/rebuild.sh green; forward-only numbered migration |
| **arch** | layer boundaries, ports, endpoint layer, proto surface | domain importing anything non-stdlib; proto or pgx types crossing into usecase; business logic in a transport | scripts/check/import-boundary.sh; buf breaking |
| **api** | proto/, connectapi, httpapi, error mapping | an error without a stable machine code; a breaking proto change without a version; an endpoint bypassing the endpoint middleware chain | buf lint + breaking; e2e suite |
| **test** | test suites, corpus files, CI gates | a bug fix without a failing-first test; a map keyed by attacker input without bound + eviction; skipped suites reported as done | `go test -race -shuffle=on` green; coverage: domain 100%, usecase ≥ 90% |

Standing truths (all profiles): a fast path that skips a check is a
vulnerability; a check that allocates per request is a regression; a cache
that ignores who asked is a data leak; any map keyed by attacker-supplied
input needs a bound and an eviction; every bug fix ships a test that fails
before and passes after, with the root cause named in one sentence.
