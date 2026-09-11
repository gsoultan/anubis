# Test timing budgets, and the graph's blind spot (2026-09-07..11)

## A short timing budget is a race against the machine

`TestTheSnapshotIsRebuiltFromScratchPeriodically` set
`m.rebuildEvery = 60 * time.Millisecond`, then asserted three `refreshAll`
calls all took the cheap path. Under `-race` on a contended CI runner those
three refreshes outlast 60 ms by themselves, `time.Since(BuiltAt) >=
rebuildEvery` goes true, and the test reports a periodic rebuild that is
**entirely correct behaviour**. It passed 12/12 locally — the signature of a
timing assumption that only holds on a quiet machine.

Fix: set the window far out of reach (`time.Hour`, matching
`TestRefreshOutcomesAreDistinguishable`) and cross it by **backdating state**
rather than sleeping. `ageSnapshots(m, by)` walks `m.data` under the manager's
own lock and replaces each pointer with a copy whose `BuiltAt` is in the past —
replaces, never mutates in place, because readers hold the `*Data` they were
handed. Deterministic, and 80 ms cheaper per run.

The rule generalises: if a test's assertion depends on how long the test
itself took, it will fail on someone else's machine. Prove it still has teeth
by breaking the production branch it guards and watching it fail.

Related: the flaky `authorize_budget_test.go` / `authz_storm_slice_test.go`
failures were proven **pre-existing** by building a database at the unmodified
0001–0037 schema and reproducing them there. Fix the methodology (best-of-N
rounds), not the budget.

## graphify silently omits this project's SQL without the extra

`graphify update` warns once and continues:

    71 .sql file(s) contributed nothing to the graph because a dependency
    is missing: tree_sitter_sql

For Anubis that is not a minor gap. ADR-0009 puts **all** SQL in
`db/queries/*.sql`, and `authorize()` — the actual authorization decision — is
SQL. A graph without it omits the load-bearing half of the system while
looking complete.

    uv tool install --force "graphifyy[sql]"   # not pip; graphify is a uv tool
    rtk graphify update .

Afterwards `graphify query "authorize"` returns `authorize()`, `grants`,
`scope_closure`, `grant_scopes`, `role_permissions_effective` from
`migrations/`. Before, none of it was reachable. File count went 181 → 750.

Note `--force` upgrades the package, which leaves the on-disk skills stale;
follow with `graphify install --platform claude` and `--platform agents`.

See [[core]].
