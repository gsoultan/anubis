-- name: GetPasswordCredential :one
SELECT id, identity_id, tenant_id, secret, params, expires_at
FROM credentials
WHERE identity_id = sqlc.arg(identity_id) AND kind = 'password' AND revoked_at IS NULL;

-- name: GetCredential :one
SELECT id, identity_id, tenant_id, kind, secret, lookup_key, label, params,
       sign_counter, created_at, last_used_at, expires_at, revoked_at
FROM credentials
WHERE id = sqlc.arg(id);

-- name: ListCredentials :many
SELECT id, identity_id, tenant_id, kind, lookup_key, label,
       sign_counter, created_at, last_used_at, expires_at, revoked_at
FROM credentials
WHERE identity_id = sqlc.arg(identity_id)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
ORDER BY created_at DESC;

-- name: CreateCredential :one
INSERT INTO credentials (identity_id, tenant_id, kind, secret, lookup_key,
                         label, params, expires_at)
VALUES (sqlc.arg(identity_id), sqlc.arg(tenant_id), sqlc.arg(kind),
        nullif(sqlc.arg(secret), ''), nullif(sqlc.arg(lookup_key), ''),
        nullif(sqlc.arg(label), ''), sqlc.arg(params)::jsonb,
        sqlc.narg(expires_at))
RETURNING id, created_at;

-- name: RevokeCredential :execrows
UPDATE credentials SET revoked_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND revoked_at IS NULL;

-- name: RevokeCredentialsOfKind :execrows
UPDATE credentials SET revoked_at = now(), updated_at = now()
WHERE identity_id = sqlc.arg(identity_id) AND kind = sqlc.arg(kind)
  AND revoked_at IS NULL;

-- name: GetCredentialByLookup :one
-- API-key auth: one index probe on credentials_lookup, never a scan.
SELECT c.id, c.identity_id, c.tenant_id, c.secret, c.expires_at,
       i.status AS identity_status, i.token_epoch,
       i.disabled_at, i.anonymized_at
FROM credentials c
JOIN identities i ON i.id = c.identity_id AND i.tenant_id = c.tenant_id
WHERE c.lookup_key = sqlc.arg(lookup_key) AND c.revoked_at IS NULL;

-- name: TouchCredentialUsed :exec
UPDATE credentials
SET last_used_at = now(),
    sign_counter = GREATEST(sign_counter, sqlc.arg(sign_counter)),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: UpdateCredentialSecret :exec
-- KDF upgrade path: rehash on next successful login (ADR-0002).
UPDATE credentials SET secret = sqlc.arg(secret), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: UpdateCredentialParams :exec
-- TOTP replay guard: params carries last accepted time step.
UPDATE credentials SET params = sqlc.arg(params)::jsonb, updated_at = now()
WHERE id = sqlc.arg(id);

-- name: ListActiveCredentialKinds :many
SELECT DISTINCT kind FROM credentials
WHERE identity_id = sqlc.arg(identity_id) AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: GetActiveCredentialOfKind :one
SELECT id, identity_id, tenant_id, kind, secret, params, sign_counter
FROM credentials
WHERE identity_id = sqlc.arg(identity_id) AND kind = sqlc.arg(kind)
  AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC
LIMIT 1;

-- name: CountActiveCredentialsOfKind :one
SELECT count(*)::int FROM credentials
WHERE identity_id = sqlc.arg(identity_id) AND kind = sqlc.arg(kind)
  AND revoked_at IS NULL;

-- name: ConsumeRecoveryCode :one
-- Single use: the row is revoked as it is accepted, in one statement, so two
-- concurrent presentations cannot both win.
UPDATE credentials
SET revoked_at = now(), last_used_at = now(), updated_at = now()
WHERE id = (
    SELECT c.id FROM credentials c
     WHERE c.identity_id = sqlc.arg(identity_id)
       AND c.kind = 'recovery_code'
       AND c.revoked_at IS NULL
       AND c.secret = sqlc.arg(code_hash)
     LIMIT 1)
RETURNING id;
