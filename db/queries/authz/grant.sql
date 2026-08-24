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

-- SearchGrants backs the Access screen.
--
-- There is deliberately no "list every grant": a tenant here holds 150k of
-- them, and a screen that asked for all of them would be answering a question
-- nobody can read. Filters narrow first, keyset paging carries the rest.
-- Ordered by (created_at, id) so the cursor is stable when two grants share a
-- timestamp — which they do, in bulk imports.
-- name: SearchGrants :many
SELECT g.id, g.identity_id, i.username, g.role_id, r.name AS role_name,
       g.self_scoped, g.valid_from, g.valid_until, g.revoked_at, g.granted_by,
       g.reason, g.via_membership_id, g.created_at
  FROM grants g
  JOIN roles r      ON r.id = g.role_id
  JOIN identities i ON i.id = g.identity_id
 WHERE g.tenant_id = sqlc.arg(tenant_id)
   AND (sqlc.arg(include_revoked)::boolean OR g.revoked_at IS NULL)
   AND (sqlc.arg(identity_id)::text = '' OR g.identity_id::text = sqlc.arg(identity_id)::text)
   AND (sqlc.arg(role_id)::text = ''     OR g.role_id::text = sqlc.arg(role_id)::text)
   -- 'direct' excludes grants derived from a membership; 'membership' keeps
   -- only those. Anything else means no filter.
   AND (sqlc.arg(source)::text <> 'direct'     OR g.via_membership_id IS NULL)
   AND (sqlc.arg(source)::text <> 'membership' OR g.via_membership_id IS NOT NULL)
   AND (sqlc.arg(query)::text = ''
        OR i.username ILIKE '%' || sqlc.arg(query)::text || '%'
        OR r.name     ILIKE '%' || sqlc.arg(query)::text || '%')
   AND (sqlc.arg(after)::text = ''
        OR (g.created_at, g.id) < (
              SELECT g2.created_at, g2.id FROM grants g2
               WHERE g2.id::text = sqlc.arg(after)::text))
 ORDER BY g.created_at DESC, g.id DESC
 LIMIT sqlc.arg(page_size);
