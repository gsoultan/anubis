-- The tenant's machine credentials (0030). Auth by single index probe; the
-- caller is the tenant's system, never a person.

-- name: CreateAPIKey :one
INSERT INTO api_keys (tenant_id, label, lookup, secret_hash, created_by, expires_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(label), sqlc.arg(lookup),
        sqlc.arg(secret_hash), sqlc.narg(created_by), sqlc.narg(expires_at))
RETURNING id;

-- name: GetAPIKeyByLookup :one
SELECT k.id, k.tenant_id, k.label, k.secret_hash, k.expires_at, k.revoked_at,
       t.slug AS tenant_slug, t.status AS tenant_status
  FROM api_keys k
  JOIN tenants t ON t.id = k.tenant_id
 WHERE k.lookup = sqlc.arg(lookup) AND k.revoked_at IS NULL;

-- ListAPIKeys shows who created each key by NAME: an audit question, and the
-- creator is a platform user, so the join goes there and nowhere near
-- identities.
-- name: ListAPIKeys :many
SELECT k.id, k.label, k.lookup, k.created_at, k.last_used_at, k.expires_at,
       k.revoked_at, COALESCE(u.username, '') AS created_by
  FROM api_keys k
  LEFT JOIN platform_users u ON u.id = k.created_by
 WHERE k.tenant_id = sqlc.arg(tenant_id)
 ORDER BY k.created_at DESC;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = now()
 WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND revoked_at IS NULL;

-- name: TouchAPIKeyUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = sqlc.arg(id);
