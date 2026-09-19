package scopermquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// UpdateSyncSource replaces a source's configuration wholesale.
//
// Wholesale, not merged: merging secrets is how half-rotated credentials
// happen. That is also why SetSyncSchedule exists — the console is never sent
// dsn or auth_header, so rescheduling through this path would save a config
// with the credentials missing.
var UpdateSyncSource = storm.SQLExec(`
UPDATE scope_sync_sources
SET config = $3::jsonb,
    status = $4,
    interval_seconds = $5,
    next_run_at = CASE
        WHEN $5 = 0 THEN NULL
        WHEN $5 <> interval_seconds OR next_run_at IS NULL THEN now()
        ELSE next_run_at
    END
WHERE id = $1 AND tenant_id = $2`)

// SourceIDRow is a new source's id.
type SourceIDRow struct {
	ID string
}

// CreateSyncSource registers a feed.
//
// A source created WITH an interval is due immediately: the operator who just
// pointed Anubis at an ERP expects the tree to fill, not to sit empty until
// the first interval elapses.
var CreateSyncSource = storm.SQL[SourceIDRow](`
INSERT INTO scope_sync_sources (tenant_id, axis_code, kind, config,
                                interval_seconds, next_run_at)
VALUES ($1, $2, $3, $4::jsonb, $5,
        CASE WHEN $5 > 0 THEN now() ELSE NULL END)
RETURNING id::text AS id`)

// SetSyncSchedule changes WHEN a source runs and nothing else.
var SetSyncSchedule = storm.SQLExec(`
UPDATE scope_sync_sources
SET interval_seconds = $3,
    next_run_at = CASE
        WHEN $3 = 0 THEN NULL
        WHEN $3 <> interval_seconds OR next_run_at IS NULL THEN now()
        ELSE next_run_at
    END
WHERE id = $1 AND tenant_id = $2`)

// RecordSyncFailure writes the run row for an attempt that never reached
// scope_sync_apply.
//
// That function opens its own row and catches per-row errors, so everything
// that gets as far as reconciling is already recorded — but a feed that cannot
// be FETCHED never gets there, and left one operator looking at an empty
// history while nothing had synced for days. The report keeps the reconciler's
// shape so one reader serves both kinds of row.
var RecordSyncFailure = storm.SQLExec(`
INSERT INTO scope_sync_runs (source_id, dry, status, finished_at, report)
VALUES ($1, false, 'failed', now(),
        jsonb_build_object(
            'error', $2::text,
            'added', 0, 'renamed', 0, 'moved', 0, 'archived', 0,
            'unchanged', 0, 'errors', '[]'::jsonb))`)

// ScheduleNextSyncSource moves a source on whether or not the run worked.
//
// A feed that is down otherwise becomes a hot loop against somebody else's
// server: the tick would find it due again a minute later, forever.
//
// It moves next_run_at ONLY. last_run_at already means something here —
// scope_sync_apply stamps it, and only after a fetch succeeded and rows were
// reconciled (0017). Stamping it here as well would make it "last attempted"
// for scheduled sources and "last succeeded" for manual ones, and the
// console's "Last synced …" line would report a time at which the sync had in
// fact failed.
var ScheduleNextSyncSource = storm.SQLExec(`
UPDATE scope_sync_sources
SET next_run_at = CASE WHEN interval_seconds > 0
                       THEN now() + make_interval(secs => interval_seconds)
                       ELSE NULL END
WHERE id = $1`)

// SyncRunRow is one entry of a feed's history.
type SyncRunRow struct {
	ID         string
	SourceID   string
	AxisCode   string
	StartedAt  time.Time
	FinishedAt runtime.Null[time.Time]
	Dry        bool
	Status     string
	Report     runtime.JSON
}

// ListSyncRuns is the history scope_sync_apply has been recording since 0017.
//
// Joined through the source so a run can only be read by the tenant that owns
// the feed — the runs table itself carries no tenant_id, so without the join
// there would be nothing to scope it by.
var ListSyncRuns = storm.SQL[SyncRunRow](`
SELECT r.id::text AS id, r.source_id::text AS source_id, s.axis_code,
       r.started_at, r.finished_at, r.dry, r.status, r.report
FROM scope_sync_runs r
JOIN scope_sync_sources s ON s.id = r.source_id
WHERE s.tenant_id = $1 AND r.source_id = $2
ORDER BY r.started_at DESC
LIMIT $3`)
