-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (session_id, tenant_id, family_id, generation,
                            token_hash, expires_at, bound_key)
VALUES (sqlc.arg(session_id), sqlc.arg(tenant_id), sqlc.arg(family_id),
        sqlc.arg(generation), sqlc.arg(token_hash), sqlc.arg(expires_at),
        nullif(sqlc.arg(bound_key), ''))
RETURNING id;

-- name: ClaimRefreshToken :one
-- The rotation core. Exactly one caller can flip active -> consumed; a second
-- presentation of the same token falls through to GetRefreshTokenByHash and
-- is recognised as theft.
UPDATE refresh_tokens
SET status = 'consumed', consumed_at = now()
WHERE token_hash = sqlc.arg(token_hash)
  AND status = 'active'
  AND expires_at > now()
RETURNING id, session_id, tenant_id, family_id, generation, expires_at;

-- name: SetRefreshSuccessor :exec
UPDATE refresh_tokens SET successor_id = sqlc.arg(successor_id)
WHERE id = sqlc.arg(id) AND expires_at = sqlc.arg(expires_at);

-- name: GetRefreshTokenByHash :one
SELECT id, session_id, tenant_id, family_id, generation, status,
       created_at, expires_at, consumed_at, revoked_at
FROM refresh_tokens
WHERE token_hash = sqlc.arg(token_hash);

-- name: RevokeRefreshFamily :execrows
-- Theft response: the whole family dies, whatever generation the attacker holds.
UPDATE refresh_tokens
SET status = 'revoked', revoked_at = now()
WHERE family_id = sqlc.arg(family_id) AND status = 'active';

-- name: RevokeRefreshBySession :execrows
UPDATE refresh_tokens
SET status = 'revoked', revoked_at = now()
WHERE session_id = sqlc.arg(session_id) AND status = 'active';

-- name: RevokeRefreshBySessions :execrows
UPDATE refresh_tokens
SET status = 'revoked', revoked_at = now()
WHERE session_id = ANY(sqlc.arg(session_ids)::uuid[]) AND status = 'active';
