-- name: ListVerificationKeys :many
-- Everything the keyring preloads: pending (published before use), active,
-- and retiring (still verifies until the last token signed with it expires).
SELECT id, kid, alg, status, purpose, public_key, private_key_enc,
       not_before, not_after, created_at
FROM signing_keys
WHERE status IN ('pending', 'active', 'retiring')
ORDER BY created_at;

-- name: ListSigningKeys :many
SELECT id, kid, alg, status, purpose, public_key, not_before, not_after,
       created_at, retired_at
FROM signing_keys
ORDER BY created_at DESC;

-- name: GetActiveSigningKey :one
SELECT id, kid, alg, status, purpose, public_key, private_key_enc,
       not_before, not_after
FROM signing_keys
WHERE purpose = sqlc.arg(purpose) AND status = 'active';

-- name: CreateSigningKey :one
INSERT INTO signing_keys (kid, alg, status, purpose, public_key,
                          private_key_enc, not_before, not_after)
VALUES (sqlc.arg(kid), sqlc.arg(alg), sqlc.arg(status), sqlc.arg(purpose),
        sqlc.arg(public_key), sqlc.arg(private_key_enc),
        sqlc.arg(not_before), sqlc.arg(not_after))
RETURNING id;

-- name: SetSigningKeyStatus :execrows
UPDATE signing_keys
SET status = sqlc.arg(status),
    retired_at = CASE WHEN sqlc.arg(status) = 'retired' THEN now() ELSE retired_at END
WHERE kid = sqlc.arg(kid);

-- name: PromotePendingKey :execrows
UPDATE signing_keys SET status = 'active'
WHERE purpose = sqlc.arg(purpose) AND status = 'pending';

-- name: DemoteActiveKey :execrows
UPDATE signing_keys SET status = 'retiring'
WHERE purpose = sqlc.arg(purpose) AND status = 'active';
