-- Platform API keys (0033): machine credentials that act as their owner.

-- name: CreatePlatformAPIKey :one
INSERT INTO platform_api_keys (platform_user_id, label, lookup, secret_hash,
                               created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at;

-- PlatformAPIKeyByLookup joins the owner so a disabled operator's key stops
-- working at the same moment they do — the status is read on every request,
-- not trusted from when the key was minted.
-- name: PlatformAPIKeyByLookup :one
SELECT k.id, k.platform_user_id, k.secret_hash, k.expires_at, k.revoked_at,
       u.username, u.status AS owner_status, u.token_epoch
FROM platform_api_keys k
JOIN platform_users u ON u.id = k.platform_user_id
WHERE k.lookup = $1 AND k.revoked_at IS NULL;

-- name: TouchPlatformAPIKeyUsed :exec
UPDATE platform_api_keys SET last_used_at = now() WHERE id = $1;

-- name: ListPlatformAPIKeys :many
SELECT k.id, k.platform_user_id, u.username, k.label, k.lookup, k.created_at,
       k.last_used_at, k.expires_at, k.revoked_at
FROM platform_api_keys k
JOIN platform_users u ON u.id = k.platform_user_id
ORDER BY k.created_at DESC;

-- name: RevokePlatformAPIKey :execrows
UPDATE platform_api_keys SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;
