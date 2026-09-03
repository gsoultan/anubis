-- The gate snapshot. ALL of these must run inside ONE REPEATABLE READ
-- read-only transaction (ADR-0005 §10): loading eight tables in separate
-- snapshots yields a torn read — a grant referencing a scope node absent from
-- the node map — a silent wrong answer, roughly weekly, unreproducible.
-- internal/snapshot.Loader owns that transaction and asserts the isolation.

-- name: SnapshotAxes :many
SELECT code, default_effect, status, sort_order FROM scope_axes
WHERE status = 'active';

-- name: SnapshotNodes :many
-- The scope hierarchy as PARENT POINTERS, not as a materialised closure.
-- The gate only ever asks "is granted node A an ancestor-or-self of target
-- B", which a walk up parent_id answers exactly -- and one row per node
-- instead of one per (node, ancestor) pair. At 1M nodes that is 1M rows
-- rather than 4M, and the in-memory form stops growing with tree depth.
--
-- NO status FILTER, DELIBERATELY. authorize() (migration 0013) probes
-- scope_closure without looking at scope_nodes.status, so an archived node
-- still carries grants and still resolves its ancestors. Filtering to
-- 'active' here would make the gate deny what the SQL engine allows, and
-- would break the chain under any archived intermediate node.
-- snapshot_parity_test.go is what catches this.
SELECT id, parent_id FROM scope_nodes
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: SnapshotGrants :many
SELECT g.id, g.identity_id, g.role_id, g.self_scoped, g.valid_from, g.valid_until
FROM grants g
WHERE g.tenant_id = sqlc.arg(tenant_id) AND g.revoked_at IS NULL;

-- name: SnapshotGrantScopes :many
SELECT gs.grant_id, gs.axis_code, gs.scope_node_id, gs.inherit
FROM grant_scopes gs
WHERE gs.tenant_id = sqlc.arg(tenant_id);

-- name: SnapshotRolePermissions :many
SELECT rpe.role_id, p.key
FROM role_permissions_effective rpe
JOIN permissions p ON p.id = rpe.permission_id
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.deprecated_at IS NULL;

-- name: SnapshotPermissions :many
SELECT p.key, p.risk, p.min_assurance, p.requires_amr,
       COALESCE(extract(epoch FROM p.max_auth_age), 0)::bigint AS max_auth_age_secs
FROM permissions p
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.deprecated_at IS NULL;

-- name: SnapshotIdentities :many
SELECT id, token_epoch, status, assurance_level,
       (disabled_at IS NOT NULL OR anonymized_at IS NOT NULL) AS blocked
FROM identities
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: SnapshotRoutes :many
SELECT rp.id, rp.priority, rp.effect, rp.path_pattern, rp.host_pattern,
       rp.methods, rp.scope_bindings, p.key AS permission_key,
       a.slug AS application_slug
FROM route_policies rp
JOIN applications a ON a.id = rp.application_id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE rp.tenant_id = sqlc.arg(tenant_id)
ORDER BY a.slug, rp.priority;

-- name: SnapshotTenants :many
SELECT id, slug FROM tenants WHERE status = 'active';

-- name: SnapshotCatalogVersion :one
-- The invalidation counter the snapshot was built from.
SELECT version, changed_at FROM catalog_version WHERE tenant_id = sqlc.arg(tenant_id);

-- name: SnapshotRevokedSessions :many
-- Revocation denylist, bounded by the longest access-token TTL: a revocation
-- older than that cannot match a still-valid token.
SELECT id FROM sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND revoked_at IS NOT NULL
  AND revoked_at > now() - sqlc.arg(revoked_window)::text::interval;
