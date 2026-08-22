# ADR-0003 — Forest of axes with closure tables

**Status:** accepted · **Date:** 2026-08-22

## Context

Access must be limitable by organisation, application, department and work
office — **and by dimensions not yet imagined** (product, customer, cost centre).
The hard requirement: *adding a dimension must not require code changes,
migrations or redeploys.*

## Rejected: fixed columns

`org_id, app_id, department_id, office_id` on the grant table. Fast and
referentially sound, but adding "cost centre" in 2027 means a schema migration, a
change to every authorization query, and a token format change. **Fails the
requirement outright.**

## Rejected: a JSONB scope bag

`scope JSONB` on grants. Infinitely extensible, but no foreign keys, no way to
express hierarchy (Jakarta *contains* Finance), unreadable queries, and no way to
answer "who has access to Jakarta?" without a full scan. **Extensible but
structurally wrong.**

## Rejected: one tree for everything

`org → office → department → team` nests cleanly. Products and customers **do
not nest under offices** — they are orthogonal. Forcing them into one hierarchy
duplicates the entire product catalog under every office, which is exactly the
combinatorial explosion the tree was meant to prevent.

## Decision: a forest of independent axes

Each axis is its own tree. A grant constrains each axis independently.

```
org axis                product axis          customer axis
  impack                  catalog               accounts
  ├── jakarta             ├── line-a            ├── enterprise
  │   ├── finance         │   ├── sku-1         │   ├── acme
  │   │   └── team-ap     │   └── sku-2         │   └── globex
  │   └── hr              └── line-b            └── smb
  └── surabaya
```

- Organisation, office, department, team are **node types on the org axis**
- Product, customer, cost centre are **separate axes**
- Adding an axis is `INSERT INTO scope_axes` — never DDL

### Semantics

1. **Within an axis:** OR — multiple nodes mean any of them
2. **Across axes:** AND — every constrained axis must match
3. **Axis absent from a grant:** governed by `scope_axes.default_effect`

Rule 3 is what makes the requirement achievable. An axis added next year is
absent from every existing grant, so **nothing breaks**. Verified empirically:
adding a `cost_center` axis with 511 nodes changed **0 of 2,000** sampled
decisions.

## Structure: closure table, not materialised path

The first design used `ltree`. Two defects surfaced:

1. **`ltree` labels are restricted to `[A-Za-z0-9_]`** — no hyphens. `line-a` is
   illegal, and so is `pt-impack-pratama`.
2. **Putting slugs in the path couples structure to naming.** Renaming a node
   rewrites every descendant's path.

| | Ancestor check | Insert leaf | Subtree move | Cycle detect | Extension |
| :--- | :--- | :--- | :--- | :--- | :--- |
| Adjacency + recursive CTE | recursive | O(1) | O(1) | recursive | none |
| Materialised path (`ltree`) | GiST scan | O(1) | **O(subtree) rewrite** | string compare | **`ltree`** |
| Nested set | fast | **O(n) renumber** | O(n) | arithmetic | none |
| **Closure table** | **PK lookup** | O(depth) | O(subtree×depth) | **O(1) probe** | **none** |

```sql
CREATE TABLE scope_closure (
    ancestor_id   uuid NOT NULL,
    descendant_id uuid NOT NULL,
    depth         smallint NOT NULL CHECK (depth >= 0),   -- 0 = self
    PRIMARY KEY (ancestor_id, descendant_id)
);
CREATE INDEX scope_closure_up ON scope_closure (descendant_id, ancestor_id)
    INCLUDE (depth);
```

"Does grant node G cover target T?" is a **primary key lookup**.

### Why this matters specifically for *dynamic* axes

- **No `CREATE EXTENSION`** — requires superuser and is blocked on many managed
  Postgres deployments. A design needing elevated privileges to function is a
  deployment liability.
- **No charset constraints** — names and structure fully decoupled.
- **`inherit=false` becomes `depth = 0`** — falls out of the model.
- **Cycle detection is one indexed probe** — reject a move if
  `EXISTS(ancestor_id = node AND descendant_id = newParent)`.
- **The planner handles it well** in multi-axis joins, where GiST estimates
  become unreliable.

Cost: O(n × depth) rows. Measured at 131,098 rows for 33,403 nodes — **71.9
bytes/row, 9 MB heap**. Trivial.

### Deliberate denormalisation

`scope_closure` omits `tenant_id` and `axis_code` even though they would permit
composite FKs, because:

- the invariant is already enforced upstream by `scope_nodes.parent_id`, and
  closure rows are written only by `scope_add_node`/`scope_move_node` — never by
  user input;
- omitting them takes the row from ~90 to ~72 bytes. More rows per 8 KB page
  means a higher cache hit ratio on the most-probed table in the system.

`depth` is `smallint`: no authorization hierarchy is 32,000 levels deep.

## Making illegal states unrepresentable

Composite foreign keys against redundant unique constraints move three
security-critical invariants from application code into the schema:

```sql
UNIQUE (id, tenant_id, axis_code),   -- redundant, but needed as an FK target

FOREIGN KEY (node_type, axis_code)
    REFERENCES scope_node_types (code, axis_code),

FOREIGN KEY (parent_id, tenant_id, axis_code)          -- same tenant AND axis
    REFERENCES scope_nodes (id, tenant_id, axis_code)
```

```sql
-- grant_scopes
FOREIGN KEY (scope_node_id, tenant_id, axis_code)
    REFERENCES scope_nodes (id, tenant_id, axis_code)
```

These make cross-tenant grants, cross-axis grants and cross-axis parenting
**impossible to insert** — not "covered by a test". Cost: one extra unique index
per table. It is the cheapest security control in the design.
[Verified](../../bench/negative.sql): 7/7 illegal writes rejected.

## Two grain corrections

**`inherit` belongs on `grant_scopes`, not `grants`.** Inheritance is per-axis.
"Everything under Jakarta, but only Product Line A itself and not its SKUs" is a
legitimate requirement a grant-level flag cannot express.

**Every axis gets an auto-created root node per tenant.** This removes an
ambiguity: without it, "unrestricted on this axis" has two encodings (no row, or
a row pointing at the root), which behave differently when the axis flips to
strict.

| Encoding | Meaning |
| :--- | :--- |
| Row pointing at the axis root | **Deliberately unrestricted** |
| No row for this axis | **Grant predates the axis** — falls under `default_effect` |

The dry-run for flipping an axis to strict can then name exactly which grants
rely on absence rather than intent.

## Where "dynamic" must stop

`scope_nodes.attributes jsonb` exists and will tempt someone to make decisions
from it. Hard line:

> **jsonb is display metadata. It is never a decision input.**

1. **Unindexable** in the general case on the hot path.
2. **Unauditable** — "who can approve for Enterprise customers?" becomes
   unanswerable.
3. **No referential integrity** — a typo'd key does not error, it silently
   matches nothing, and `false` in an authorization system is an outage nobody
   can diagnose.

Anything participating in a decision becomes an axis. That is what the axis
registry is *for*.

## Consequences

**Positive** — new dimensions are three API calls; hierarchy works within each
axis; reorganisation is a subtree move; time-boxed grants come free; cross-tenant
leaks are structurally impossible.

**Negative** — more tables than fixed columns; multi-axis queries need care (see
[ADR-0005](0005-database-performance.md)); five axes make grants genuinely hard
for humans to reason about.

**Mitigation** — build for N axes, **launch with two** (`org`, `app`). Add others
only when a concrete requirement demands one. The architecture should be open;
the configuration should stay disciplined. Ship `/v1/authorize/explain` in the
same phase — with multiple axes it is not optional.
