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
