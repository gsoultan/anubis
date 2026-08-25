-- Platform refresh tokens (0031): single-use, family-revoked, hash-only.

-- InsertPlatformRefreshRoot begins a family: the row's own id IS the family
-- id, which is what lets one revocation kill a sign-in however many times
-- it rotated.
-- name: InsertPlatformRefreshRoot :one
INSERT INTO platform_refresh_tokens (id, platform_user_id, family_id, token_hash, expires_at)
SELECT g.u, $1, g.u, $2, $3 FROM (SELECT uuidv7() AS u) g
RETURNING id;

-- name: InsertPlatformRefreshChild :one
INSERT INTO platform_refresh_tokens (platform_user_id, family_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: PlatformRefreshByHash :one
SELECT id, platform_user_id, family_id, created_at, expires_at, used_at, revoked_at
FROM platform_refresh_tokens
WHERE token_hash = $1;

-- ConsumePlatformRefresh is the rotation's atomic heart: exactly one caller
-- can flip used_at, and a second concurrent presenter gets zero rows — which
-- the interactor treats as reuse, not as a race to shrug at.
-- name: ConsumePlatformRefresh :execrows
UPDATE platform_refresh_tokens
SET used_at = now()
WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL;

-- name: RevokePlatformRefreshFamily :execrows
UPDATE platform_refresh_tokens
SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: SweepPlatformRefresh :execrows
DELETE FROM platform_refresh_tokens
WHERE expires_at < now() - interval '1 day';
