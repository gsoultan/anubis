# Upgrading storm (2026-09-08, v0.2.0 → v0.10.0)

`cmd/stormgen` is this module's storm tool — five lines wrapping `tool.Main`,
because storm's commands are a library that must see *this* module's models.
A binary installed from storm's own repo cannot.

## The procedure

    export ANUBIS_DB_URL=postgres://anubis:anubis@127.0.0.1:7449/anubis?sslmode=disable
    go get github.com/gsoultan/storm@vX.Y.Z && go mod tidy
    go run ./cmd/stormgen generate internal/authz/adapter/postgres/rgen \
      -raw-schema live -dsn "$ANUBIS_DB_URL"

`-raw-schema live` is required and deliberate: `rmodel` is a **projection**,
`migrations/` is the schema of record, and the raw declarations call SQL
functions (`authorize`, `membership_*`) the model does not describe.
Validating against a scratch apply of the model would fail every one.

CI (`scripts/ci/backend-suite.sh`) regenerates and fails on any diff.

## The verify baseline — memorise these three numbers

    verify -stale     clean
    verify -pending   1
    verify (drift)    59

`-pending` and drift are **not** regressions. They are the documented
consequence of `rmodel` projecting `migrations/` rather than defining it —
drift wants to `DROP TABLE tenants` because the model only describes `roles`.
Measure them at the old version *and* the new one rather than trusting a
previous commit message; if either moves, something real changed.

## What a clean upgrade looks like

Generated diff is three files, one line each — the version stamp. Anything
more deserves reading.

## Soft delete (v0.8.0–v0.10.0) is inert here

No model calls `t.SoftDelete` and the schema has no `deleted_at` column
anywhere, so generated output is byte-identical across the whole range. **If a
table ever opts in**, two placements pass review looking fine and are wrong:

- A **joined** table's predicate belongs in its `ON` clause. In a `WHERE` it
  silently turns a `LEFT JOIN` into an inner one — a parent whose only child
  is deleted disappears entirely. The *driving* table's predicate does belong
  in the `WHERE`.
- A **recursive CTE** must be guarded in both halves. Guard only the anchor
  and a deleted row re-enters on the second iteration, bringing its whole
  subtree.

## Verify at runtime, not just compile time

Generated code that compiles proves less than generated code that runs. Boot
`anubisd` and confirm snapshots load with unchanged grant counts, and exercise
a storm read path (`ListMemberships`, `ListPermissions`) against a live
database. There are no DB-backed Go unit tests for authz — that path is
covered only by the e2e suite.

`go-sql-driver/mysql` is a **direct** dependency via `cmd/anubisd/syncengines`,
not transitive through storm. It looks wrong in a Postgres-only binary and
isn't.

See [[core]].
