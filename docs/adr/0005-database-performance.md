# ADR-0005 — Database design for performance

**Status:** accepted · **Date:** 2026-08-22
**Measured on:** PostgreSQL 18.4 (aarch64, Apple Container), 4 vCPU / 4 GB,
`shared_buffers=1GB`, dataset of 150,000 grants · 269,491 grant_scopes ·
128,056 closure rows · 50,000 identities · 32,381 scope nodes.

---

## 1. UUIDv7 primary keys, never UUIDv4

Postgres 18 provides `uuidv7()` natively. It is time-ordered, so index inserts
land on the **rightmost B-tree page** instead of scattering across the whole
index.

| | UUIDv4 | UUIDv7 |
| :--- | :--- | :--- |
| Insert locality | random — page splits everywhere | sequential — rightmost page |
| Cache behaviour | hot set is the entire index | hot set is a few leaf pages |
| WAL volume | high — full-page images from splits | low |

On the insert-heavy tables (`audit_log`, `refresh_tokens`, `sessions`) this is
the single largest win available, and it costs one word in the DDL.

```sql
id uuid PRIMARY KEY DEFAULT uuidv7()
```

## 2. Column order follows alignment

Postgres pads columns to their alignment boundary. Declaring 8-byte types first,
then 4-byte, then 1-byte and varlena removes dead padding.

```sql
CREATE TABLE sessions (
    created_at   timestamptz NOT NULL,   -- 8-byte aligned, first
    last_seen_at timestamptz NOT NULL,
    auth_time    timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    id           uuid PRIMARY KEY,       -- 16 bytes, 1-byte alignment
    identity_id  uuid NOT NULL,
    ...
    amr          text[],                 -- varlena last
);
```

On wide, high-row-count tables this is 10–20% fewer pages, which is 10–20% less
I/O and more rows resident in cache.

## 3. `scope_closure` is deliberately narrow

The most-probed table in the system. `tenant_id` and `axis_code` are **omitted**
despite permitting composite FKs, because the invariant is already enforced by
`scope_nodes.parent_id` and closure rows are written only by
`scope_add_node`/`scope_move_node` — never by user input.

```sql
CREATE TABLE scope_closure (
    ancestor_id   uuid NOT NULL,
    descendant_id uuid NOT NULL,
    depth         smallint NOT NULL,   -- not int: no tree is 32k deep
    PRIMARY KEY (ancestor_id, descendant_id)
) WITH (fillfactor = 95);
```

**Measured: 71.9 bytes/row, 9,096 kB heap for 131,098 rows.** Carrying tenant and
axis would add ~25%, meaning proportionally fewer rows per 8 KB page and a lower
cache hit ratio on the hottest table.

## 4. The traversal direction — where measurement overruled analysis

Three formulations of the multi-axis check were benchmarked on **3,000 decisions
where grants sit at broad scope nodes** (segment ≈ 4,000 descendants, axis root
≈ 20,000) — the ordinary admin, auditor and regional-manager case.

| Formulation | Time | Result |
| :--- | ---: | :--- |
| **A.** downward — expand the granted node's descendants | 284 ms | ✅ |
| **B.** upward — expand the target's ancestors via CTE join | **4,976 ms** | ✅ |
| **C.** upward — correlated `EXISTS` driven by `grant_scopes` | **142 ms** | ✅ |

All three returned **identical results on every row** (0 disagreements).

### The trap

Isolated `EXPLAIN` strongly favoured **B**: expanding a target's ancestors touches
~4 rows (bounded by tree *depth*), while expanding a granted node's descendants
touches up to 20,106 (bounded by *subtree size*). Measured in isolation:

```
downward:  756 shared buffers, 1.5 MB hash of the entire subtree
upward:      8 shared buffers, Index Only Scan, both columns bound
```

**A 94× reduction — and B was still 17× slower end to end.** The cause:

```
Function Scan on jsonb_each_text  (cost=0.00..1.00 rows=100)  (actual rows=2)
```

`jsonb_each_text()` has no statistics, so Postgres assumes 100 rows against an
actual 2. That 50× overestimate propagates into the join above it and the planner
abandons the nested loop for a hash/materialise strategy.

**Letting an unestimatable function scan *drive a join* costs far more than any
traversal saves.**

### Why C wins

The closure probe is correlated to `gs.scope_node_id` — a real column on a real
table with real statistics. Both closure columns become equality-bound, giving a
two-column probe on the `(ancestor_id, descendant_id)` primary key. `EXISTS`
short-circuits on the first hit, so broad grants cost the same as narrow ones.
`MATERIALIZED` pins the function scan where its bad estimate cannot influence
plan choice.

```sql
WITH targets AS MATERIALIZED (          -- pin it; do not let it drive a join
    SELECT key AS axis_code, value::uuid AS node_id FROM jsonb_each_text(p_targets)),
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id      -- both columns
                      AND c.ancestor_id   = gs.scope_node_id  -- equality-bound
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0)) AS satisfied
      FROM grant_scopes gs JOIN candidates cd ON cd.id = gs.grant_id)
```

> **`EXPLAIN` on an isolated subquery misled us. Only the end-to-end workload
> exposed the regression. Benchmark the decision, not the subquery.**

This reasoning is reproduced in `migrations/0007_authorize.sql` so nobody
"optimises" it back.

## 5. Partial and covering indexes

```sql
CREATE INDEX grants_identity_live ON grants (identity_id)
    INCLUDE (role_id, id, valid_from, valid_until)
    WHERE revoked_at IS NULL;
```

`WHERE revoked_at IS NULL` is always present in the hot query, so the index need
not carry dead rows — fewer levels, fewer buffer reads. `INCLUDE` makes the
candidate lookup an **index-only scan**; the heap is never touched.

`scope_closure_up (descendant_id, ancestor_id) INCLUDE (depth)` similarly
enables index-only ancestor scans — `depth` is needed for `inherit=false` and
would otherwise force a heap fetch.

Measured index usage over a 20,000-decision run:

```
grants_id_tenant_id_key      270,219 scans   7,288 kB
scope_closure_pkey           184,197 scans   9,312 kB
grant_scopes_pkey            118,656 scans      18 MB
rpe_permission                97,200 scans      88 kB
permissions_key               97,068 scans      40 kB
grants_identity_live          97,017 scans      12 MB
scope_closure_up              61,463 scans      11 MB
```

## 6. Partition the churning tables from day one

| Table | Growth | Strategy |
| :--- | :--- | :--- |
| `audit_log` | unbounded, high write | **Range-partition by month** |
| `refresh_tokens` | one row per refresh, forever | **Range-partition by `expires_at`** |
| `login_attempts` | very high write, low value | Redis counters; persist aggregates only |

Retention becomes `DROP PARTITION` — instant — instead of a bulk `DELETE` that
leaves dead tuples for autovacuum to grind through. `refresh_tokens` has the
highest churn in the system; getting this wrong is how auth databases become
400 GB of bloat.

**Retrofitting partitioning requires a full table rewrite under `ACCESS
EXCLUSIVE`.** On an auth service that means downtime for every application at
once. It is free today and expensive later.

`DEFAULT` partitions catch anything outside the provisioned range, so an insert
can never fail because a scheduled job did not run.

`audit_log` uses **BRIN** on `occurred_at`: the table is append-only and
physically time-ordered, so BRIN is orders of magnitude smaller than the
equivalent B-tree while answering range scans just as well.

## 7. Dynamic axes are rows, never DDL

Partitioning `scope_nodes` by `axis_code` is tempting — adding an axis would
create a partition. **Rejected.** It means the admin API issues DDL at runtime,
requiring `CREATE` on the schema for the application role. Giving an auth service
the ability to create and drop tables is a far worse trade than marginal
partition pruning.

> **If adding an axis requires a schema change, the requirement is not met. If
> you solve that by granting DDL rights to the application, you have traded a
> deploy for a privilege-escalation path.**

## 8. Statement-level triggers, not row-level

The first implementation used `FOR EACH ROW` for catalog invalidation. A bulk
sync of a 20,000-node customer tree would then fire 20,000 `UPDATE`s against a
**single** `catalog_version` row — self-inflicted lock contention on one hot
tuple — plus 20,000 `pg_notify()` calls, which pass through a fixed-size queue
that **fails the entire transaction** when full.

Statement-level triggers with transition tables collapse that to one bump per
statement regardless of row count. Invalidation semantics are unchanged: readers
care only that the version moved, not how far.

`catalog_version` uses `fillfactor = 70` — it is updated constantly and needs
free page space for HOT updates.

## 9. Generated columns over trigger-maintained denormalisation

`permissions.key` was originally set by a `BEFORE INSERT` trigger. It came out
**silently NULL** under `session_replication_role = 'replica'` — which applies to
bulk loads and logical replication apply.

> A denormalisation a trigger keeps correct is only as reliable as the trigger
> being enabled.

```sql
app_slug text NOT NULL,
key      text GENERATED ALWAYS AS
         (app_slug || ':' || resource || ':' || action) STORED,

FOREIGN KEY (application_id, app_slug)
    REFERENCES applications(id, slug) ON UPDATE CASCADE
```

`key` is now maintained by the storage engine. `app_slug` cannot drift, because
the composite FK permits no other value and `ON UPDATE CASCADE` rewrites these
rows when an application is renamed. Zero triggers, zero drift possible.

## 10. Snapshot loads use `REPEATABLE READ`

The hot path serves from an in-memory snapshot spanning eight tables. Loading
them in separate queries yields a **torn read** — a grant referencing a scope
node absent from the node map, because a sync committed between query three and
query four. The result is a silent deny (or with sloppy nil handling, a silent
allow), roughly weekly, and unreproducible.

```go
tx, _ := db.BeginTx(ctx, &sql.TxOptions{
    Isolation: sql.LevelRepeatableRead,
    ReadOnly:  true,
})
```

Not optional.

---

## Results

| Metric | Value |
| :--- | :--- |
| Authorization decisions | **20,000 in 909 ms → 0.045 ms each, ~22,000/sec** (single connection) |
| Correctness suite | 5/5, including fail-closed |
| Schema-enforced invariants | **7/7 illegal writes rejected by the database** |
| Runtime axis addition | 511 nodes, **0 of 2,000 decisions changed** |
| `scope_closure` row width | 71.9 bytes |
| Total database size | 164 MB |

| Table | Rows | Heap | Indexes | Total |
| :--- | ---: | ---: | ---: | ---: |
| `grants` | 150,000 | 21 MB | 25 MB | 46 MB |
| `grant_scopes` | 314,570 | 24 MB | 20 MB | 44 MB |
| `scope_closure` | 131,098 | 9 MB | 20 MB | 29 MB |
| `identities` | 50,000 | 7.7 MB | 13 MB | 20 MB |
| `scope_nodes` | 33,403 | 5.4 MB | 7.1 MB | 12 MB |
