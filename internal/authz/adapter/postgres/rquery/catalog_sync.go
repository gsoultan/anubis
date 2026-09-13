package authzrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// CatalogSourceRow is a configured place an application's catalog comes
// from, with the application's slug joined on: every caller needs it (the
// apply path is scoped by slug) and it cannot drift from the pin.
type CatalogSourceRow struct {
	ID              string
	TenantID        string
	ApplicationID   string
	ApplicationSlug string
	Kind            string
	Format          string
	Status          string
	Name            string
	Config          string
	IntervalSeconds int32
	LastRunAt       runtime.Null[time.Time]
	NextRunAt       runtime.Null[time.Time]
	// How the most recent attempt ended. A list of sources exists to answer
	// "which feed is broken", and last_run_at alone cannot: a source that
	// failed an hour ago and one that applied an hour ago read identically.
	LastStatus runtime.Null[string]
}

const catalogSourceCols = `
       s.id::text AS id, s.tenant_id::text AS tenant_id,
       s.application_id::text AS application_id, a.slug AS application_slug,
       s.kind, s.format, s.status, s.name, s.config::text AS config,
       s.interval_seconds, s.last_run_at, s.next_run_at,
       (SELECT r.status FROM catalog_sync_runs r
         WHERE r.source_id = s.id
         ORDER BY r.started_at DESC LIMIT 1) AS last_status`

// ListCatalogSources is one tenant's sources, newest configuration last so
// the list reads in the order somebody built it.
var ListCatalogSources = storm.SQL[CatalogSourceRow](`
SELECT` + catalogSourceCols + `
FROM catalog_sync_sources s
JOIN applications a ON a.id = s.application_id
WHERE s.tenant_id = $1
ORDER BY s.name`)

var GetCatalogSource = storm.SQL[CatalogSourceRow](`
SELECT` + catalogSourceCols + `
FROM catalog_sync_sources s
JOIN applications a ON a.id = s.application_id
WHERE s.id = $1 AND s.tenant_id = $2`)

// DueCatalogSources is the scheduler's whole query and crosses tenants on
// purpose — the job is one loop for the installation, not one per tenant.
// A source with no next_run_at is manual and is not selected by the partial
// index behind this.
var DueCatalogSources = storm.SQL[CatalogSourceRow](`
SELECT` + catalogSourceCols + `
FROM catalog_sync_sources s
JOIN applications a ON a.id = s.application_id
WHERE s.status = 'active'
  AND s.next_run_at IS NOT NULL
  AND s.next_run_at <= $1
ORDER BY s.next_run_at
LIMIT $2`)

// CreatedCatalogSourceRow is a fresh source's id.
type CreatedCatalogSourceRow struct{ ID string }

// CreateCatalogSource takes the first run's due time from the interval, so a
// scheduled source starts polling without waiting for a whole period to pass
// before anyone finds out the URL was wrong.
var CreateCatalogSource = storm.SQL[CreatedCatalogSourceRow](`
INSERT INTO catalog_sync_sources
       (tenant_id, application_id, kind, format, status, name, config,
        interval_seconds, next_run_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8,
        CASE WHEN $8 > 0 THEN now() ELSE NULL END)
RETURNING id::text AS id`)

var UpdateCatalogSource = storm.SQLExec(`
UPDATE catalog_sync_sources
SET name = $3, status = $4, config = $5::jsonb, format = $6,
    interval_seconds = $7,
    -- Changing the interval reschedules from now; leaving it alone leaves
    -- the existing due time exactly where it was.
    next_run_at = CASE
        WHEN $7 = 0 THEN NULL
        WHEN $7 <> interval_seconds OR next_run_at IS NULL THEN now()
        ELSE next_run_at
    END,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

// ScheduleCatalogSource moves a source past the run that just happened.
// Computed from now() rather than from the previous due time: a source whose
// feed took four minutes should wait its whole interval afterwards, not fire
// again immediately because the clock caught up.
var ScheduleCatalogSource = storm.SQLExec(`
UPDATE catalog_sync_sources
SET last_run_at = now(),
    next_run_at = CASE WHEN interval_seconds > 0
                       THEN now() + make_interval(secs => interval_seconds)
                       ELSE NULL END
WHERE id = $1`)

// DeleteCatalogSource removes a source and, by cascade, its run history.
// The applies themselves survive in the audit log — what is lost is the
// record of the fetches, which is the right trade for a source somebody
// created by mistake.
var DeleteCatalogSource = storm.SQLExec(`
DELETE FROM catalog_sync_sources WHERE id = $1 AND tenant_id = $2`)

// CatalogRunRow is one entry in a source's history.
type CatalogRunRow struct {
	ID          string
	SourceID    string
	StartedAt   time.Time
	FinishedAt  runtime.Null[time.Time]
	Dry         bool
	Status      string
	Actor       string
	DocumentSha string
	Report      runtime.Null[string]
	Error       string
}

// StartCatalogRun opens the history entry BEFORE the fetch. A run that dies
// mid-fetch leaves a 'running' row, which is the only way anybody finds out
// that a feed hangs.
var StartCatalogRun = storm.SQL[CreatedCatalogSourceRow](`
INSERT INTO catalog_sync_runs (source_id, tenant_id, dry, actor)
VALUES ($1, $2, $3, $4)
RETURNING id::text AS id`)

var FinishCatalogRun = storm.SQLExec(`
UPDATE catalog_sync_runs
SET finished_at = now(), status = $2, document_sha = $3,
    report = nullif($4, '')::jsonb, error = $5
WHERE id = $1`)

var ListCatalogRuns = storm.SQL[CatalogRunRow](`
SELECT id::text AS id, source_id::text AS source_id, started_at, finished_at,
       dry, status, actor, document_sha, report::text AS report, error
FROM catalog_sync_runs
WHERE source_id = $1
ORDER BY started_at DESC
LIMIT $2`)

// LastAppliedDigestRow carries the digest of the document a source last
// acted on, which is what makes an unchanged catalog a no-op.
type LastAppliedDigestRow struct{ DocumentSha string }

// LastAppliedDigest ignores dry runs: a dry run proves a document parses, it
// does not mean the rows are in the database.
var LastAppliedDigest = storm.SQL[LastAppliedDigestRow](`
SELECT document_sha
FROM catalog_sync_runs
WHERE source_id = $1 AND dry = false AND status IN ('ok','skipped')
      AND document_sha <> ''
ORDER BY started_at DESC
LIMIT 1`)
