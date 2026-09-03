# Benchmarks

All figures reproduced by `./bench/rebuild.sh`, roughly 30 seconds end to end.

**Environment** — PostgreSQL 18.4 (aarch64-alpine) on Apple Container 1.2.2,
macOS 26.5, Apple Silicon. 4 vCPU / 4 GB. `shared_buffers=1GB`,
`effective_cache_size=3GB`, `work_mem=32MB`, `random_page_cost=1.1`.

**Dataset**

| | Rows |
| :--- | ---: |
| Grants | 150,000 |
| Grant scope constraints | 269,751 |
| Closure edges | 128,138 |
| Identities | 57,000 across 3 realms |
| Scope nodes | 33,403 across 4 axes |
| Permissions / roles | 200 / 53 |

---

## Results

```
==> dropping and rebuilding schema
    10 migrations applied
==> seeding
    150000 grants, 269751 grant_scopes, 128138 closure rows,
    57000 identities across 3 realms
==> correctness
     exact scope match -> ALLOW        | t   | t
     inherited descendant -> ALLOW     | t   | t
     different subtree -> DENY         | f   | f
     permission not held -> DENY       | f   | f
     FAIL-CLOSED: axis omitted -> DENY | f   | f
     cross-tenant identity -> DENY     | f   | f
==> external populations (suppliers, applicants)
     supplier reads own company PO -> ALLOW             | t   | t
     supplier reads ANOTHER company PO -> DENY          | f   | f
     applicant reads OWN record -> ALLOW                | t   | t
     applicant reads SOMEONE ELSE record -> DENY        | f   | f
     FAIL-CLOSED: self-scoped, no _owner -> DENY        | f   | f
     ASSURANCE: IAL1 applicant, IAL3 permission -> DENY | f   | f
     disabled identity -> DENY | f   | f
     anonymised (retention) identity -> DENY | f   | f
     same username across 3 realms |   3 |    3
    2/2 escalation attempts rejected
==> negative (all must be blocked by the schema)
    7/7 illegal writes rejected
==> performance
    20k decisions: Time: 891.963 ms
```

**Throughput: 20,000 decisions in 892 ms → 0.045 ms each, ~22,400/sec** on a
single connection, each decision resolving the grant's full multi-axis target
map.

---

## Storage

| Table | Rows | Heap | Indexes | Total | Bytes/row |
| :--- | ---: | ---: | ---: | ---: | ---: |
| `grants` | 150,000 | 21 MB | 25 MB | 46 MB | 149 |
| `grant_scopes` | 269,751 | 24 MB | 20 MB | 44 MB | 93 |
| `scope_closure` | 128,138 | 9 MB | 20 MB | 29 MB | **71.9** |
| `identities` | 57,000 | 7.7 MB | 13 MB | 20 MB | 158 |
| `scope_nodes` | 33,403 | 5.4 MB | 7.1 MB | 12 MB | 168 |

Total database: **164 MB**.

`scope_closure` at 71.9 bytes/row is the deliberate outcome of omitting
`tenant_id` and `axis_code` — the invariant is enforced upstream by
`scope_nodes.parent_id`, and rows are written only by the maintenance functions.
It is the most-probed table in the system, so rows-per-page directly drives cache
hit ratio.

## Index usage over a 20,000-decision run

```
grants_id_tenant_id_key      270,219 scans   7,288 kB
scope_closure_pkey           184,197 scans   9,312 kB
grant_scopes_pkey            118,656 scans      18 MB
rpe_permission                97,200 scans      88 kB
permissions_key               97,068 scans      40 kB
grants_identity_live          97,017 scans      12 MB
scope_closure_up              61,463 scans      11 MB
```

`grants_id_tenant_id_key` is the most-scanned index in the system — it is the
composite-FK target that makes cross-tenant grants impossible. The security
control is also on the hot path, which is the right place for it to be.

---

## The traversal-direction experiment

Three formulations of the multi-axis check, benchmarked on **3,000 decisions
where grants sit at broad scope nodes** (segment ≈ 4,000 descendants, axis root
≈ 20,000) — the ordinary admin, auditor and regional-manager case.

| Formulation | Time | Agreement |
| :--- | ---: | :--- |
| **A.** downward — expand the granted node's descendants | 284 ms | ✅ |
| **B.** upward — expand the target's ancestors via CTE join | **4,976 ms** | ✅ |
| **C.** upward — correlated `EXISTS` driven by `grant_scopes` | **142 ms** | ✅ |

Zero disagreements across all 3,000 rows.

**Isolated `EXPLAIN` strongly favoured B:**

```
downward:  756 shared buffers, 1.5 MB hash of the entire subtree
upward:      8 shared buffers, Index Only Scan, both columns bound
```

A 94× buffer reduction — and B was still **17× slower end to end**. The cause:

```
Function Scan on jsonb_each_text  (cost=0.00..1.00 rows=100)  (actual rows=2)
```

`jsonb_each_text()` has no statistics. Postgres assumes 100 rows against an
actual 2, and that 50× overestimate propagates into the join above it, so the
planner abandons the nested loop for a hash/materialise strategy.

**Letting an unestimatable function scan drive a join costs more than any
traversal saves.**

C correlates the closure probe to `gs.scope_node_id` — a real column with real
statistics — making both closure columns equality-bound (a two-column PK probe),
short-circuiting on first hit, with `MATERIALIZED` pinning the function scan
where its estimate cannot influence plan choice.

> **`EXPLAIN` on an isolated subquery is not evidence. Benchmark the decision,
> not the subquery.**

---

## Dynamic axis addition

Registering a `cost_center` axis at runtime — three INSERTs, no DDL, no
migration, no restart — then bulk-syncing 511 nodes from a simulated ERP:

| Measure | Result |
| :--- | :--- |
| Schema changes required | **0** |
| Nodes synced | 511 |
| Decisions replayed | 2,000 |
| **Decisions changed** | **0** |

Dry-run of flipping the axis to `default_effect='deny'`: allows drop from
**800 → 0**, because no grant constrains the new axis yet. Exactly the signal
needed before making that change in production.

---

## The gate snapshot at a million scope nodes

Measured through the real `LoadSnapshot` against a single tenant of
1,010,101 `scope_nodes` / 4,030,201 `scope_closure` rows.

The SQL engine barely notices the size — the closure probe is a two-column PK
lookup, so thirty times the data costs thirty percent more time:

| | 32k nodes | 1M nodes |
| :--- | :--- | :--- |
| `authorize()` allow | 0.045 ms | **0.059 ms** |
| `authorize()` deny | — | **0.039 ms** |

The gate's in-memory copy was the real ceiling. It held the whole transitive
closure as nested string-keyed maps; it now holds parent pointers and walks
them ([ADR-0015](adr/0015-scope-hierarchy-at-scale.md)):

| | closure map | parent index |
| :--- | :--- | :--- |
| snapshot resident | 530.7 MB | **91.9 MB** |
| per node | 550.9 B | 95.4 B |
| live heap objects | 1,014,000 | **4,245** |
| load | 1613 ms | ~340 ms |
| `Evaluate` (1 grant, allow) | 73.0 ns | 76.7 ns |
| `Evaluate` (20 grants, allow) | 329.4 ns | 113.7 ns |

Two things are worth reading off that table rather than the headline.

**Memory is now flat in depth** — 75 B/node at depth 3 and at depth 11, where
the closure form went from 296 to 527 B/node over the same range. The old
shape grew with the tree; this one does not, which is what makes a deep
hierarchy safe rather than merely affordable today.

**The object count moved further than the megabytes.** The driver allocates a
separate string per node id, so a million-node snapshot kept a million extra
pointers for the GC to trace on every scan — paid in request latency, not RSS.
Slabbing the ids into one string took 1,014,000 live objects to 4,245 at a
cost of ~20 ms on a load that happens on refresh and never on the request
path.

The single-grant `Evaluate` case is ~4 ns slower; the multi-grant cases are
2–3× faster, because the old shape paid a map probe per grant per axis while
the walk pays one lookup and then integer loads. Both are far inside the
sub-microsecond budget ADR-0005 §4 sets for the decision itself.

### Chain depth is quadratic, and that is the real bound

`scope_closure` stores one row per (node, ancestor) pair, so a *chain* costs
n²/2 rows:

| chain depth | closure rows | cumulative build |
| :--- | :--- | :--- |
| 250 | 31,626 | 0.5 s |
| 500 | 125,751 | 1.8 s |
| 1,000 | 501,501 | 7.5 s |
| 2,000 | 2,003,001 | 26 s |

The declared ceiling is the `smallint` bound on `depth`, 32,767 — but that
would be ~537M closure rows, so the practical limit arrives long before the
declared one. Migration 0038 raises a named `program_limit_exceeded` instead
of letting this surface as "smallint out of range". Anything approaching these
numbers is a self-referencing external feed, not a hierarchy.

## Benchmark hygiene

Four rules, each learned by getting it wrong first.

**1. Benchmark the decision, not the subquery.** See above.

**2. Compare variants identical in every other respect.** An early comparison
omitted the strict-axis clause from one variant. The result was meaningless and
had to be discarded.

**3. Verify variants agree before comparing speed.**

```sql
SELECT count(*) FILTER (WHERE a <> b) AS disagreements
FROM (SELECT variant_a(...) a, variant_b(...) b FROM workload) x;
```

**4. Include the pathological case.** Broad grants are the common case for
admins and auditors, not an edge case. A workload of only narrow grants showed
284 ms vs 685 ms — a *wash* — and hid a 35× difference entirely.

**5. Make probes deterministic.** An early correctness probe took the first grant
it found via `LIMIT 1` and supplied only the `org` target. When the seed grew, it
selected a grant that also constrained `product`, and the correct fail-closed
deny looked like a regression. Probes now build their target map from the grant's
own constraints.
