# Scope sync on a clock (0045)

0017 gave a structure a source of truth and a reconciler but no timer: every
sync ran because somebody pressed a button, so an org chart was only as current
as the last person who remembered it. 0045 adds the same two columns the
catalog side already had (0043), deliberately identical so the two schedulers
cannot drift into two behaviours.

## The shape, and why it is a copy

`scope_sync_sources.next_run_at` + `interval_seconds`, a partial index over
`(next_run_at) WHERE status='active' AND next_run_at IS NOT NULL`, and a
`scope_sync` job in `cmd/anubisd/maintenance.go`.

- **A minute is the TICK, not the interval.** Each source carries its own, with
  a 300s floor in the CHECK *and* in `validateSyncConfig` — the schema makes it
  impossible, the app tier makes it a message instead of a constraint error.
- **Its own advisory lock** (`lockScopeSync`), not catalog's. Sharing one would
  serialise every tenant's structure refresh behind every tenant's catalog
  refresh, and one slow ERP would starve the other job entirely.
- **A tick that finds nothing costs one index probe.** Verified: forcing the
  plan gives `Index Scan using scope_sync_sources_due`, `Index Cond:
  (next_run_at <= now())`, with the status/NOT NULL predicate absorbed by the
  partial index. At four rows the planner picks a seq scan, correctly.

## The three things that would have been bugs

- **Reschedule in a `defer`, before the run.** A source moves on whether or not
  the run worked. Without it a feed that is down is found due again one minute
  later, forever — a hot loop against somebody else's server. `runDueOne` exists
  only so the defer has a scope to hang on. Failing to reschedule logs at ERROR,
  not WARN, because that failure *is* the hot loop.
- **A scheduled run must not wear an operator's identity.** `emitSync` takes a
  tenant id and a `*authctx.Principal` that is nil for the timer; it then audits
  `actor_kind='system'` with no actor id. `runSync` takes `tenantID` as its own
  parameter rather than reading `p.TenantID`, so the scheduled path *cannot*
  fall back to an ambient tenant it does not have.
- **An empty feed must archive nothing.** This matters more here than for a
  catalog: a catalog feed returning nothing applies nothing, but a SCOPE feed
  returning nothing means "every node you have is gone" and the reconciler would
  archive the whole axis. `RunSync` already refused it; the scheduled path goes
  through the same function, so the refusal was inherited rather than
  re-implemented. 0045 adds no way around it.

All three have tests in `internal/scope/app/run_due_test.go` that were confirmed
to fail against the code without them.

A fourth, caught late: **the reschedule must move `next_run_at` only.**
`last_run_at` already has a meaning here — `scope_sync_apply` stamps it, and
only after a fetch succeeded (0017:153). Stamping it in the reschedule too made
it "last attempted" for scheduled sources and "last succeeded" for manual ones,
so the console's "Last synced 15:01" would name a time at which the sync had in
fact failed. The catalog's equivalent does set it; scope's must not, because
scope already had the column doing something else.

## A failure before the reconciler used to leave no trace

`scope_sync_apply` opens its own run row and catches per-row errors, so
anything that gets as far as reconciling records itself. A fetch that fails
never gets there — and before the scheduler that was tolerable, because a human
had pressed the button and saw the error. Unattended it meant the Source pane
showed a schedule, an empty history, and no hint that nothing had synced for
days. Verified on the dev box: `e2e_dead_axis` had **0 rows** in
`scope_sync_runs` after real failed attempts.

`RecordSyncFailure` writes that row from the app tier, with the reason in the
report under the same shape the reconciler uses so one reader serves both. It
covers the two pre-reconciler exits: an unreachable feed, and the zero-row
refusal. A run that *does* reach the reconciler must not get a second row —
there is a test for that, because it is the obvious way to break this later.

The console distinguishes them: a run that reconciled and could not place N
rows shows "N unplaced", one that never reached the feed shows why. The second
used to render as "0 unplaced".

## Changing the schedule of a source that already exists

`SetSyncSchedule` is its own RPC, usecase and query. It cannot go through
`UpdateSyncSource`, which replaces `config` wholesale on purpose — and no
client is ever sent a source's `dsn` or `auth_header`, so a console screen
rescheduling through that path would save the source with its credentials
gone. The query touches `interval_seconds` and `next_run_at` and nothing else.

Its CASE is shared with `UpdateSyncSource` and verified against the database:
setting an interval schedules from now; setting the *same* interval again keeps
the existing due time (so an unrelated edit does not restart the clock); 0
clears `next_run_at`; and anything between 1 and 299 is refused by the CHECK.

## Keeping RunDue off the transport

`ScopeAdminService` **embeds** `ScopeAdminUsecase`, so anything added to the
sync usecase lands on the RPC surface. `RunDue` therefore lives on its own
interface, `ScopeSyncSchedulerUsecase`, and `NewScopeAdminInteractor` returns
the composite `ScopeAdmin`. The service constructor still takes the narrow
`ScopeAdminUsecase`, so `scopeAdminService` has no `RunDue` on it at all — the
transport is handed a type without the method, not a rule saying not to call it.
`scripts/check/import-boundary.sh` already greps for `.RunDue(` in any adapter,
so the name gets that protection for free. Same reasoning as
[[catalog-sync]]'s `ApplyDocumentAsSystem`.

## Watch out

`internal/scope/domain/` sits at exactly 10 Go files, the `folder-size.sh`
limit, and **test files count**. That is why the tenant went onto
`SyncSourceRecord` rather than into a new `DueSyncSource` type — which turned
out better anyway: the scheduler holds the tenant as a value instead of
assuming one.

## Related

[[catalog-sync]] · [[console-design-system]]
