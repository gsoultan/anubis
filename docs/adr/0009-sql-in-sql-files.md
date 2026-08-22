# ADR-0009 — All SQL lives in .sql files; sqlc compiles it

**Status:** accepted · **Date:** 2026-08-22

## Context

The project rule is *no SQL in Go code*. SQL is reviewed, benchmarked and
owned as SQL (`migrations/`, `bench/`); the application layer must not grow a
second, string-concatenated dialect of it. The rule needs a mechanism, not a
convention.

## Decision

1. **Every query is authored in `db/queries/*.sql`** as a named
   [sqlc](https://sqlc.dev) query. `migrations/*.sql` is sqlc's schema input,
   so **every query is type-checked against the real schema at generation
   time** — column drift is a build failure, not a runtime surprise.
2. sqlc generates pgx/v5 code into `internal/adapter/postgres/gen/`
   (committed; CI regenerates and fails on drift). The only runtime
   dependency is `pgx`, already accepted by ADR-0002.
3. `internal/adapter/postgres` is the only package that may execute queries.
   The database-side engine stays database-side: `authorize()` and
   `authorize_explain()` are called through one-line sqlc wrappers.
4. **Enforcement:** `scripts/check/no-sql-in-go.sh` fails CI if SQL keywords
   appear in string literals in any hand-written `.go` file.

## The two exemptions, stated rather than smuggled

| Package | Why it may hold SQL |
| :--- | :--- |
| `internal/migrate` | The runner (hand-written per ADR-0002) executes SQL *before* the schema it would be generated against exists. Its statements are three fixed lines against `schema_migrations`. |
| `internal/repository/feed` | Scope-sync sources read **foreign** databases (`kind = db_query \| db_table`, migrations/0017) over their own `config.dsn` connection. Those schemas are unknown at build time, so sqlc cannot type-check them and `db/queries` cannot host them. |

The feed exemption is bounded by construction: `db_query` executes the
operator's own configured query verbatim, and `db_table` assembles a
`SELECT` only from identifiers validated against `^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`
and quoted through `pgx.Identifier.Sanitize()`. Neither path can touch
Anubis's own schema — a different connection, a different database.
Everything reading Anubis's tables still goes through `db/queries`.

## The tooling line ADR-0002 implies

ADR-0002 governs **what links into the shipped binary**. Codegen and analysis
tools never ship, so they are toolchain, not dependencies:

| Tool | Version | Role |
| :--- | :--- | :--- |
| `sqlc` | 1.31.x | .sql → typed pgx code (build time) |
| `buf` + `protoc-gen-go` + `protoc-gen-connect-go` | current | proto → Go/TS (build time) |

Versions are recorded here and checked by `scripts/gen.sh`; generated output
is committed so builds never require the tools.

## Consequences

**Positive** — SQL reviewed as SQL next to the migrations that define it;
compile-time schema checking; zero new runtime dependencies; the ADR-0005
performance discipline (EXPLAIN on the real query text) applies directly to
the files the app executes.

**Negative** — generated code in-tree (mitigated by drift check); dynamic
filters must be expressed as static queries with nullable parameters, which
occasionally means one more query than an ORM would need. Accepted — that is
the auditable shape.
