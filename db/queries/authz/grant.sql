-- name: ListGrantsByIdentity :many
SELECT g.id, g.identity_id, g.role_id, r.name AS role_name, g.self_scoped,
       g.valid_from, g.valid_until, g.revoked_at, g.granted_by, g.reason,
       g.via_membership_id
FROM grants g
JOIN roles r ON r.id = g.role_id
WHERE g.identity_id = sqlc.arg(identity_id) AND g.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(include_revoked)::boolean OR g.revoked_at IS NULL)
ORDER BY g.created_at DESC;

-- name: ListGrantScopes :many
SELECT gs.grant_id, gs.axis_code, gs.scope_node_id, gs.inherit,
       sn.name AS node_name
FROM grant_scopes gs
JOIN scope_nodes sn ON sn.id = gs.scope_node_id
WHERE gs.grant_id = ANY(sqlc.arg(grant_ids)::uuid[])
ORDER BY gs.grant_id, gs.axis_code, sn.name;

-- name: CreateGrant :one
INSERT INTO grants (tenant_id, identity_id, role_id, granted_by, reason,
                    self_scoped, valid_until)
VALUES (sqlc.arg(tenant_id), sqlc.arg(identity_id), sqlc.arg(role_id),
        sqlc.arg(granted_by), nullif(sqlc.arg(reason), ''),
        sqlc.arg(self_scoped), sqlc.narg(valid_until))
RETURNING id, valid_from;

-- name: InsertGrantScope :exec
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit)
VALUES (sqlc.arg(grant_id), sqlc.arg(tenant_id), sqlc.arg(axis_code),
        sqlc.arg(scope_node_id), sqlc.arg(inherit));

-- name: RevokeGrant :one
UPDATE grants
SET revoked_at = now(),
    reason = CASE WHEN sqlc.arg(reason) = '' THEN reason ELSE sqlc.arg(reason) END
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND revoked_at IS NULL
RETURNING id, identity_id, role_id;
