-- name: ListScopeAxes :many
SELECT code, display_name, default_effect, status, sort_order,
       resolution, ui_schema
FROM scope_axes
ORDER BY sort_order, code;

-- name: GetScopeAxis :one
SELECT code, display_name, default_effect, status, sort_order,
       resolution, ui_schema
FROM scope_axes
WHERE code = sqlc.arg(code);

-- name: CreateScopeAxis :one
INSERT INTO scope_axes (code, display_name, default_effect, sort_order,
                        resolution, ui_schema)
VALUES (sqlc.arg(code), sqlc.arg(display_name), sqlc.arg(default_effect),
        sqlc.arg(sort_order), sqlc.arg(resolution)::jsonb,
        sqlc.arg(ui_schema)::jsonb)
RETURNING code;

-- name: UpdateScopeAxis :execrows
UPDATE scope_axes
SET display_name = sqlc.arg(display_name),
    default_effect = sqlc.arg(default_effect),
    status = sqlc.arg(status),
    sort_order = sqlc.arg(sort_order),
    ui_schema = sqlc.arg(ui_schema)::jsonb
WHERE code = sqlc.arg(code);

-- name: ListScopeNodeTypes :many
SELECT code, axis_code, display_name, parent_types
FROM scope_node_types
WHERE sqlc.narg(axis_code)::text IS NULL OR axis_code = sqlc.narg(axis_code)
ORDER BY axis_code, code;

-- name: CreateScopeNodeType :one
INSERT INTO scope_node_types (code, axis_code, display_name, parent_types)
VALUES (sqlc.arg(code), sqlc.arg(axis_code), sqlc.arg(display_name),
        sqlc.arg(parent_types)::text[])
RETURNING code;

-- name: ListScopeNodes :many
-- KEYSET paging, ordered by (name, id). Resume by passing the last row's
-- name and id as after_name/after_id; NULL after_name starts at the
-- beginning. Not OFFSET: at a million nodes OFFSET re-scans everything it
-- skips, and it drops or repeats rows when a sync inserts ahead of the
-- cursor. name is not unique, which is why id is in both the ORDER BY and
-- the comparison. Index: scope_nodes_paging (migration 0039).
SELECT id, tenant_id, parent_id, is_axis_root, status, axis_code, node_type,
       slug, name, external_ref
FROM scope_nodes
WHERE tenant_id = sqlc.arg(tenant_id)
  AND axis_code = sqlc.arg(axis_code)
  AND (sqlc.narg(parent_id)::uuid IS NULL OR parent_id = sqlc.narg(parent_id))
  AND (sqlc.narg(query)::text IS NULL OR name ILIKE '%' || sqlc.narg(query) || '%')
  AND (sqlc.arg(include_archived)::boolean OR status = 'active')
  AND (sqlc.narg(after_name)::text IS NULL
       OR name > sqlc.narg(after_name)::text
       OR (name = sqlc.narg(after_name)::text AND id > sqlc.narg(after_id)::uuid))
ORDER BY name, id
LIMIT sqlc.arg(lim);

-- name: GetScopeNode :one
SELECT id, tenant_id, parent_id, is_axis_root, status, axis_code, node_type,
       slug, name, external_ref
FROM scope_nodes
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: GetScopeNodeByRef :one
SELECT id, tenant_id, parent_id, is_axis_root, status, axis_code, node_type,
       slug, name, external_ref
FROM scope_nodes
WHERE tenant_id = sqlc.arg(tenant_id) AND axis_code = sqlc.arg(axis_code)
  AND external_ref = sqlc.arg(external_ref);

-- name: EnsureAxisRoot :one
SELECT scope_ensure_root(sqlc.arg(tenant_id), sqlc.arg(axis_code)) AS node_id;

-- name: AddScopeNode :one
SELECT scope_add_node(sqlc.arg(tenant_id), sqlc.arg(axis_code),
                      sqlc.arg(node_type), sqlc.arg(parent_id),
                      sqlc.arg(slug), sqlc.arg(name),
                      nullif(sqlc.arg(external_ref), '')) AS node_id;

-- name: MoveScopeNode :exec
SELECT scope_move_node(sqlc.arg(node_id), sqlc.arg(new_parent_id));

-- name: ArchiveScopeNode :execrows
UPDATE scope_nodes SET status = 'archived', updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND NOT is_axis_root;

-- name: RenameScopeNode :execrows
UPDATE scope_nodes SET name = sqlc.arg(name), status = 'active', updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: ScopeSyncApply :one
SELECT scope_sync_apply(sqlc.arg(source_id), sqlc.arg(rows)::jsonb,
                        sqlc.arg(dry))::text AS report;

-- name: ListSyncSources :many
SELECT id, tenant_id, axis_code, kind, status, config, last_run_at,
       interval_seconds, next_run_at
FROM scope_sync_sources
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY axis_code;

-- name: GetSyncSource :one
SELECT id, tenant_id, axis_code, kind, status, config, last_run_at,
       interval_seconds, next_run_at
FROM scope_sync_sources
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: UpdateSyncSource :execrows
UPDATE scope_sync_sources
SET config = sqlc.arg(config)::jsonb,
    status = sqlc.arg(status),
    interval_seconds = sqlc.arg(interval_seconds),
    -- Changing the interval reschedules from now; leaving it alone leaves the
    -- existing due time exactly where it was, so saving an unrelated edit does
    -- not quietly restart the clock on a feed that was about to run.
    next_run_at = CASE
        WHEN sqlc.arg(interval_seconds) = 0 THEN NULL
        WHEN sqlc.arg(interval_seconds) <> interval_seconds OR next_run_at IS NULL THEN now()
        ELSE next_run_at
    END
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: CreateSyncSource :one
INSERT INTO scope_sync_sources (tenant_id, axis_code, kind, config, interval_seconds,
                                next_run_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(axis_code), sqlc.arg(kind),
        sqlc.arg(config)::jsonb, sqlc.arg(interval_seconds),
        -- A source created WITH an interval is due immediately: the operator
        -- who just pointed Anubis at an ERP expects the tree to fill, not to
        -- sit empty until the first interval elapses.
        CASE WHEN sqlc.arg(interval_seconds) > 0 THEN now() ELSE NULL END)
RETURNING id;

-- RecordSyncFailure writes the run row for an attempt that never reached
-- scope_sync_apply. That function opens its own row and catches per-row errors,
-- so everything that gets as far as reconciling is already recorded — but a
-- feed that cannot be fetched never gets there, and left one operator looking
-- at an empty history while nothing had synced for days. The report keeps the
-- reconciler's shape so one reader serves both kinds of row.
-- name: RecordSyncFailure :exec
INSERT INTO scope_sync_runs (source_id, dry, status, finished_at, report)
VALUES (sqlc.arg(source_id), false, 'failed', now(),
        jsonb_build_object(
            'error', sqlc.arg(reason)::text,
            'added', 0, 'renamed', 0, 'moved', 0, 'archived', 0,
            'unchanged', 0, 'errors', '[]'::jsonb));

-- SetSyncSchedule changes WHEN a source runs and nothing else. It exists
-- because UpdateSyncSource replaces config wholesale — which is correct, since
-- merging secrets is how half-rotated credentials happen — and the console is
-- never sent dsn or auth_header. Rescheduling through that path would save a
-- config with the credentials missing.
-- name: SetSyncSchedule :execrows
UPDATE scope_sync_sources
SET interval_seconds = sqlc.arg(interval_seconds),
    next_run_at = CASE
        WHEN sqlc.arg(interval_seconds) = 0 THEN NULL
        WHEN sqlc.arg(interval_seconds) <> interval_seconds OR next_run_at IS NULL THEN now()
        ELSE next_run_at
    END
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- DueSyncSources is the scheduler's only read. It carries no tenant filter
-- because a timer serves every tenant at once; that is exactly why the usecase
-- behind it must never be reachable from a transport.
-- name: DueSyncSources :many
SELECT id, tenant_id, axis_code, kind, status, config, last_run_at,
       interval_seconds, next_run_at
FROM scope_sync_sources
WHERE status = 'active'
  AND next_run_at IS NOT NULL
  AND next_run_at <= sqlc.arg(now)
ORDER BY next_run_at
LIMIT sqlc.arg(lim);

-- ScheduleNextSyncSource moves a source on whether or not the run worked. A
-- feed that is down otherwise becomes a hot loop against somebody else's
-- server: the tick would find it due again one minute later, forever.
--
-- It moves next_run_at ONLY. last_run_at already has a meaning in this schema
-- — scope_sync_apply stamps it, and only after a fetch succeeded and rows were
-- reconciled (0017). Stamping it here too would make it "last attempted" for
-- scheduled sources and "last succeeded" for manual ones, and the console's
-- "Last synced …" line would cheerfully report a time at which the sync had in
-- fact failed.
-- name: ScheduleNextSyncSource :exec
UPDATE scope_sync_sources
SET next_run_at = CASE WHEN interval_seconds > 0
                       THEN now() + make_interval(secs => interval_seconds)
                       ELSE NULL END
WHERE id = sqlc.arg(id);

-- ScopeAncestors is the chain from an axis root down to a node, which is what
-- makes a scope decision explainable: "this grant reaches here BECAUSE it was
-- given on that ancestor". Read from the closure table, so it costs one index
-- scan rather than a recursive walk.
-- name: ScopeAncestors :many
SELECT n.id, n.axis_code, n.node_type, n.parent_id, n.slug, n.name,
       n.external_ref, n.status, n.is_axis_root, c.depth
  FROM scope_closure c
  JOIN scope_nodes n ON n.id = c.ancestor_id
 WHERE c.descendant_id = sqlc.arg(node_id)
   AND n.tenant_id = sqlc.arg(tenant_id)
 ORDER BY c.depth DESC;

-- Dashboard: the structure's live size.
-- name: CountActiveScopeNodes :one
SELECT count(*) FROM scope_nodes WHERE tenant_id = $1 AND status = 'active';

-- ScopeNodesByIDs resolves a HANDFUL of nodes by id — the names beside the
-- grants on one screen. The console used to pull every node in every axis
-- (32k here) to render a dozen labels.
-- name: ScopeNodesByIDs :many
SELECT id, tenant_id, parent_id, is_axis_root, status, axis_code, node_type,
       slug, name, external_ref
FROM scope_nodes
WHERE tenant_id = sqlc.arg(tenant_id) AND id = ANY(sqlc.arg(ids)::uuid[]);

-- ListSyncRuns is the history scope_sync_apply has been recording since
-- 0017 and nothing ever read. Joined through the source so a run can only
-- be read by the tenant that owns the feed — the runs table itself carries
-- no tenant_id.
-- name: ListSyncRuns :many
SELECT r.id, r.source_id, s.axis_code, r.started_at, r.finished_at, r.dry,
       r.status, r.report
FROM scope_sync_runs r
JOIN scope_sync_sources s ON s.id = r.source_id
WHERE s.tenant_id = sqlc.arg(tenant_id) AND r.source_id = sqlc.arg(source_id)
ORDER BY r.started_at DESC
LIMIT sqlc.arg(lim);
