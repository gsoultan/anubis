-- name: CreateOneTimeToken :one
INSERT INTO one_time_tokens (tenant_id, kind, token_hash, payload, expires_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(kind), sqlc.arg(token_hash),
        sqlc.arg(payload)::jsonb, sqlc.arg(expires_at))
RETURNING id;

-- name: ConsumeOneTimeToken :one
-- Atomic single use: DELETE ... RETURNING has GETDEL semantics. A second
-- presentation finds no row. GET-then-DEL would leave a replay window.
DELETE FROM one_time_tokens
WHERE token_hash = sqlc.arg(token_hash)
  AND kind = sqlc.arg(kind)
  AND expires_at > now()
RETURNING id, tenant_id, payload;

-- name: SweepOneTimeTokens :execrows
DELETE FROM one_time_tokens WHERE expires_at < now() - interval '1 hour';
