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
SELECT id, tenant_id, parent_id, is_axis_root, status, axis_code, node_type,
       slug, name, external_ref
FROM scope_nodes
WHERE tenant_id = sqlc.arg(tenant_id)
  AND axis_code = sqlc.arg(axis_code)
  AND (sqlc.narg(parent_id)::uuid IS NULL OR parent_id = sqlc.narg(parent_id))
  AND (sqlc.narg(query)::text IS NULL OR name ILIKE '%' || sqlc.narg(query) || '%')
  AND (sqlc.arg(include_archived)::boolean OR status = 'active')
ORDER BY name
LIMIT 2000;

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
SELECT id, tenant_id, axis_code, kind, status, config, last_run_at
FROM scope_sync_sources
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY axis_code;

-- name: GetSyncSource :one
SELECT id, tenant_id, axis_code, kind, status, config, last_run_at
FROM scope_sync_sources
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: UpdateSyncSource :execrows
UPDATE scope_sync_sources
SET config = sqlc.arg(config)::jsonb,
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: CreateSyncSource :one
INSERT INTO scope_sync_sources (tenant_id, axis_code, kind, config)
VALUES (sqlc.arg(tenant_id), sqlc.arg(axis_code), sqlc.arg(kind),
        sqlc.arg(config)::jsonb)
RETURNING id;
