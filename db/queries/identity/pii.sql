-- name: CreatePIIKey :one
INSERT INTO pii_keys (tenant_id, key_enc, kms_key_ref)
VALUES (sqlc.arg(tenant_id), sqlc.arg(key_enc), nullif(sqlc.arg(kms_key_ref), ''))
RETURNING id;

-- name: GetPIIKey :one
SELECT id, tenant_id, key_enc FROM pii_keys
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: ShredPIIKey :one
-- Idempotent: a second erasure of the same identity returns false rather than
-- failing, because "already unrecoverable" is the requested outcome.
SELECT pii_shred(sqlc.arg(key_id), sqlc.arg(reason)) AS shredded;

-- name: SetIdentityPIIKey :exec
UPDATE identities SET pii_key_id = sqlc.arg(pii_key_id), updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: GetIdentityAttributes :one
-- The envelope and the key that opens it, read together: fetching them in two
-- statements leaves a window where retention shreds the key in between and the
-- caller reports "corrupt" for what is actually a completed erasure.
SELECT i.attributes, i.pii_key_id, k.key_enc
FROM identities i
LEFT JOIN pii_keys k ON k.id = i.pii_key_id
WHERE i.id = sqlc.arg(id) AND i.tenant_id = sqlc.arg(tenant_id);

-- name: SetIdentityAttributes :execrows
UPDATE identities SET attributes = sqlc.arg(attributes), updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);
