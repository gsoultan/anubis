# ADR-0015 — The gate snapshot at a million scope nodes

**Status:** accepted · **Date:** 2026-08-31
**Measured on:** PostgreSQL 18 (aarch64, Apple Container), 4 vCPU / 4 GB,
a single tenant of 1,010,101 `scope_nodes` / 4,030,201 `scope_closure` rows,
through the real `LoadSnapshot`.

---

## The question

Can Anubis carry a million scope nodes? The answer split in two, and only one
half was ever in doubt.

`authorize()` was never the problem. The closure probe is a two-column PK
lookup, so it scales logarithmically:

| | 32k nodes | 1M nodes |
| :--- | :--- | :--- |
| allow | 0.045 ms | **0.059 ms** |
| deny | — | **0.039 ms** |

Thirty times the data cost thirty percent more time. The database was fine.

The gate was not. `snapshot.Data.Up` held the tenant's **entire transitive
closure** in memory as `map[string]map[string]int16` — one nested map per
node, keyed by 36-character UUID strings.

## Decision

Store parent pointers and walk them.

The evaluator's only question is "does granted node A cover target B" —
ancestor-or-self — and `depth` was compared against nothing but `0`. Parent
pointers answer that exactly, in O(nodes) instead of O(nodes × depth).

| | before | after |
| :--- | :--- | :--- |
| snapshot resident | 530.7 MB | **91.9 MB** |
| per node | 550.9 B | 95.4 B |
| live heap objects | 1,014,000 | **4,245** |
| load | 1613 ms | ~340 ms |

Memory is now **flat in depth** — 75 B/node at depth 3 and at depth 11, where
the closure form went from 296 to 527 B/node over the same range. That
flatness, not the absolute number, is the property worth keeping.

The object count matters more than the megabytes. The driver allocates a
separate string per id; a million of those are a million extra pointers for
the GC to trace on every scan, which is paid in request latency rather than in
RSS. `NewScopeIndex` copies the ids into one string and keys the map with
slices of it.

`scope_closure` still exists and `authorize()` still uses it. The two are now
**independent derivations of the same relation**, which is what makes
`snapshot_parity_test.go` worth running: before, both sides read the same
table and agreement proved little.

## Consequences that are easy to get wrong

**`authorize()` does not filter `scope_nodes.status`.** Neither may
`SnapshotNodes`. Adding `AND status = 'active'` is the obvious tidy-up and it
breaks every ancestor chain that passes through an archived node, making the
gate deny what the SQL engine allows — a partial outage for one subtree that
nobody attributes to a scope archive three weeks earlier.

**An un-interned constraint must deny.** `ScopeConstraint.node` stores the
index **plus one**, because a plain `0` is a valid index: a missed
`InternGrantScopes()` would otherwise match whichever node loaded first. That
is the one place a fail-open default is unaffordable.

**The ancestor walk is bounded.** A cycle in `parent[]` is unreachable through
the API — `parent_id` is written only by `scope_move_node`, which probes the
closure first — but the closure map this replaced could not loop at all, and
this runs per grant per axis on every decision. Running out of steps denies.

## The second ceiling: tenant count

Reducing per-tenant memory only moved the limit. `Manager.data` holds every
tenant's snapshot on every instance, and the poll rebuilt all of them every
30 seconds whether or not anything had changed.

Making that conditional on the catalog version looked easy and was unsound:
the version was bumped for six tables, while the snapshot read five more that
bumped nothing — `sessions`, `identities`, `scope_axes`, `applications` and
`role_permissions_effective`. Gating on the version would have silently
stopped propagating **revocation, identity blocking and strict-axis flips**.

Migration 0040 closes all five, so `Manager.load` now probes the version
(0.3 ms) and rebuilds only when it moved. Revocation gained from this too: it
rides the push path now instead of waiting for the poll.

The triggers are deliberately narrow, and the narrowness cuts both ways.
`sessions.last_seen_at` is written on **every authenticated request** and
`identities.last_login_at` on every login; a trigger that fired on those would
make snapshot rebuild a per-request cost, far worse than the polling it
replaced. They are also **statement level**, per ADR-0005 and migration 0006:
`RevokeAllSessions` is one bulk `UPDATE`, and a row-level trigger would fire
one `catalog_version` upsert and one `pg_notify` per session.

### The gate does not trust itself indefinitely

A missing trigger is invisible at runtime. The version simply never moves, the
rebuild is skipped forever, and the gate serves stale authorization with
nothing in the logs to say so — the failure looks exactly like a genuinely
idle tenant.

So the skip is bounded: if a snapshot has not been rebuilt from scratch within
`ANUBIS_SNAPSHOT_MAX_AGE`, the next refresh rebuilds regardless of the
version. That is deliberately the SAME knob that defines staleness, so the
guarantee is one operators already reason about — *nothing is served that has
not been fully rebuilt within max age* — rather than a second, subtly
different freshness bound to hold in your head. At the default 30 s poll and
5 min max age it still skips about nine rebuilds in ten.

`Data` therefore carries two timestamps: `LoadedAt` (last confirmed current,
which the version gate can refresh without rebuilding) and `BuiltAt` (last
actually rebuilt). Readiness judges `LoadedAt`; the periodic rebuild judges
`BuiltAt`.

`anubis_gate_snapshot_refresh_total{result}` makes all of this visible —
`unchanged` should dominate, `verify` should appear once per tenant per
max-age window, and its **absence** is the alert, because that is what a lost
trigger looks like from outside.

This invariant is load-bearing and not locally visible, so it is pinned by
tests at three levels — structural (every snapshot table is classified against
actual `pg_trigger` state), behavioural (decisions must bump, hot paths must
not), and end-to-end (revoke a real session, confirm the rebuilt snapshot
denies it). Each was verified to fail when the `sessions` trigger is dropped.

## Validated on a running server

Left idle against the dev dataset for five minutes, three tenants loaded:

```
anubis_gate_snapshot_refresh_total{tenant="impack",result="rebuilt"}    1
anubis_gate_snapshot_refresh_total{tenant="impack",result="unchanged"} 12
anubis_gate_snapshot_refresh_total{tenant="impack",result="verify"}     1
```

One initial build, twelve polls skipped, and the periodic rebuild firing once
at the max-age mark exactly as designed — the mechanism behaves in a live
process the way the unit tests describe it with a fake loader.

RSS told us something the load-time benchmarks could not: a rebuild spikes to
396 MB and settles to 21 MB within thirty seconds. Both copies of a snapshot
are live until the pointer swap, which is inherent to keeping readers
lock-free. Sizing an instance to its idle RSS therefore invites an OOM-kill on
the next rebuild — see [operations](../operations.md). It is also a second,
unplanned argument for the version gate: skipping a rebuild skips the spike.

## What is still open

Per-tenant memory is ~92 MB at a million nodes, held for every tenant on every
instance. The next lever is lazy per-tenant loading with an LRU, which trades
cold-start latency on a tenant's first request against memory — and puts the
database back on the request path, which ADR-0005 rules out. Sharding tenants
across instances costs no gate code at all and should be preferred until the
numbers say otherwise.

Depth is separately capped at the `smallint` ceiling of 32,767 (migration
0038 now raises a named `program_limit_exceeded` rather than "smallint out of
range"). The closure is quadratic in chain length — depth 2,000 is already
2,003,001 rows and ~26 s to build — so that bound is nowhere near the real
budget. A chain that long is a self-referencing external feed, not a
hierarchy.
