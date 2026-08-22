-- Per-tenant sign-in page configuration (migrations/0018).

-- name: GetSigninPage :one
SELECT tenant_id, config, updated_at FROM signin_pages
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: PutSigninPage :exec
INSERT INTO signin_pages (tenant_id, config, updated_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(config)::jsonb, now())
ON CONFLICT (tenant_id) DO UPDATE
    SET config = EXCLUDED.config, updated_at = now();
