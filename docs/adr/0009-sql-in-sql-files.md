# ADR-0009 — All SQL lives in .sql files; sqlc compiles it

**Status:** accepted, amended §5 · **Date:** 2026-08-22 · **Amended:** 2026-08-25

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

## §7 Amendment (2026-09-18): the model is the schema of record

`internal/platform/schema` declares the whole database — 45 tables, 28
functions, 38 triggers, one view, both partitioned tables — and
`cmd/stormddl` turns a change to it into the next numbered file in
`migrations/`. §5.3's "storm's DDL generation is not used" no longer holds:
it is used, and it is the source of truth.

**What did NOT change.** `migrations/` is still forward-only, still
checksummed, still what runs, and still reviewed as SQL before it is applied.
`anubisd migrate` is unchanged. `stormddl` writes a migration and applies
nothing, which is the whole difference between this and an automigrate.
Migrations 0001–0046 predate the model and stay exactly as they are — the model
describes the schema they produce and owns everything after them.

**Why the model is one package and not seven.** Each context's `rmodel` is a
PROJECTION: it models the tables that context queries and declares another
context's tables as plain columns, because `scripts/check/context-boundary.sh`
forbids one context's adapter importing another's. A composite foreign key that
carries a tenant across that line — grants → identities → realms → tenants —
cannot be declared from inside either context. So there are two kinds of model
and one schema: this package is the schema, the rmodels are views onto it.

**Why two models is safe here.** Neither can drift from the database without CI
saying so. `stormddl -check` fails if the schema of record and the live schema
disagree — in either direction: a model edited without a migration, or a
migration applied without the model following it. `cmd/stormgen` PREPAREs every
context's raw queries against that same schema. Both are anchored to the
database, so they can only differ in the way a projection is allowed to, by
describing less.

**The exemption this adds.** `no-sql-in-go.sh` exempts
`internal/platform/schema`, whose function and trigger bodies are PL/pgSQL by
definition. That SQL still reaches `migrations/` and is still reviewed there
before it runs.

## §6 Amendment (2026-09-17): the audit context, and builders over raw SQL

The audit context moves off sqlc. Unlike authz — which migrated as raw
`storm.SQL` declarations against one generated model — audit is **model-first**:
`audit_log` is declared in `rmodel/`, storm generates the builders, and the
repository composes predicates instead of naming a statement per filter
combination.

That is the difference worth recording. `QueryAudit` had five optional filters,
each written as `($n::uuid IS NULL OR actor_id = $n)` because a `.sql` file
cannot omit a clause. A builder omits it: an absent filter contributes no
predicate, and storm compiles one statement per *shape*, so the shapes anybody
actually uses stay prepared. The dynamic query stops being a static query
wearing a disguise.

What stays raw, in `rquery/`, is what genuinely is SQL: two void-returning
function calls (`ensure_month_partitions`), the per-tenant advisory lock, and
two aggregates using `FILTER`. Five declarations, against nine `.sql` queries
before.

`cmd/stormgen` now reads the bounded context out of the output path and hands
storm only that context's models, so a query cannot compile against a table its
context does not own — the boundary `AGENTS.md` draws, enforced where the code
is produced. `scripts/ci/backend-suite.sh` regenerates every storm context and
fails on drift.

The storm executor and the uuid conversions moved to `platform/database`. They
are plumbing, not policy, and one copy per context is one copy per context to
forget when `database.Conn` grows a third producer.

The **control** context followed on the same pattern (2026-09-17): four tables
it exclusively owns, builders for the CRUD, and raw declarations for the joins
and for the guarded updates that set a SERVER-side expression — `revoked_at =
now()` is not the same fact on a client clock, and `token_epoch = token_epoch +
1` computed in Go is a read-modify-write that loses an increment under two
concurrent disables.

Those two SET forms were also, at the time, things storm could not say. As of
storm v0.16.0 it can — `SetRevokedAtNow`, `IncTokenEpoch`, and `MutateKey` for
the caller with nothing to read first — and the sign-in lookup moved to a
builder with `EqLower`, which lowers to the `lower(username)` the unique index
is built on. What keeps the guarded updates in SQL now is their WHERE, not
their SET: they address rows by something other than the primary key, and the
guard has to be in the statement because a read-then-write is the window it
exists to close. That distinction matters for the next reader — a note naming
a constraint that has since lifted sends them looking in the wrong place.

The **tenancy** context followed (2026-09-17): five tables it owns, builders
for the reads and writes that stay on one table, raw declarations for the LEFT
joins that resolve an auth page's binding, the interval renderings, the
predicate DELETEs storm has no builder form for, and `signin_pages` — which is
a VIEW, and storm models tables.

**scope** and **platform** followed (2026-09-17). scope is mostly SQL and that
is the right shape for it: the tree's writes are database FUNCTIONS —
scope_add_node, scope_move_node, scope_ensure_root, scope_sync_apply — because
a move rewrites the closure table and re-checks the type rules in one
statement, and the closure table is what authorize() reads, so a half-applied
move is a wrong ANSWER rather than a slow one.

platform has no models at all: two advisory locks, which belong to no table.
They are SESSION-scoped, so they run on one pinned connection —
`database.Executor` maps `*pgxpool.Conn` onto storm's connection adapter, added
upstream in v0.15.0 for exactly this. Released through the pool, the lock would
be released on whichever connection came back next, which releases nothing.

**Still sqlc:** identity, auth, gate. The registry in `cmd/stormgen` is the
record of how far this has got.

## §5 Amendment (2026-08-25): storm in the authz context

The authz context is migrating from sqlc to [storm](https://github.com/gsoultan/storm)
(M6 of storm's plan; anubis is the first adopter). The *rule* — SQL is
reviewed in one designated place per context and checked against the real
schema before it can build — is unchanged. The *mechanism* gains a second
sanctioned form:

1. **Builder queries carry no SQL at all.** `rgen/` packages are generated
   from the model in `rmodel/`; their SQL text is emitted by storm's
   compiler, structurally injection-safe, and never hand-edited.
2. **Raw SQL lives in exactly one package per context:**
   `internal/<context>/adapter/postgres/rquery/`, as `storm.SQL[T]`
   declarations. `cmd/stormgen` PREPAREs every declaration against the live
   dev schema at generation time — column drift or type drift fails the
   *build*, preserving the property sqlc gave `db/queries/*.sql`.
3. The model in `rmodel/` is a **projection** of the schema; `migrations/`
   remains the schema of record. storm's DDL generation is not used.
4. `scripts/check/no-sql-in-go.sh` now also catches backtick SQL bodies and
   exempts only `rquery/` (designated), `gen/`/`rgen/` (generated), and the
   two §-exemptions above — so `storm.SQL` cannot quietly spread beyond the
   designated file.

While the migration is in flight, sqlc's `gen/` and storm's `rgen/` coexist
in the authz context; the sqlc side shrinks as call sites move.

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
