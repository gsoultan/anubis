-- name: GetIdentityForLogin :one
-- The login lookup. LEFT JOIN so an identity predating realm assignment still
-- resolves; realm policy fields ride along to avoid a second round trip.
SELECT i.id, i.tenant_id, i.token_epoch, i.status, i.username, i.email,
       i.assurance_level, i.disabled_at, i.anonymized_at, i.realm_id,
       r.code AS realm_code, r.kind AS realm_kind
FROM identities i
LEFT JOIN realms r ON r.id = i.realm_id
WHERE i.tenant_id = sqlc.arg(tenant_id)
  AND i.realm_id = sqlc.arg(realm_id)
  AND lower(i.username) = lower(sqlc.arg(username));

-- name: GetIdentity :one
SELECT i.id, i.tenant_id, i.token_epoch, i.status, i.username, i.email,
       i.external_ref, i.assurance_level, i.disabled_at, i.anonymized_at,
       i.created_at, i.last_login_at, i.realm_id, i.category_id,
       r.code AS realm_code, r.kind AS realm_kind,
       c.code AS category_code
FROM identities i
LEFT JOIN realms r ON r.id = i.realm_id
LEFT JOIN realm_categories c ON c.id = i.category_id
WHERE i.id = sqlc.arg(id) AND i.tenant_id = sqlc.arg(tenant_id);

-- name: ListIdentities :many
-- Keyset pagination on id: uuidv7 is time-ordered, so id order is creation
-- order and the probe stays an index range scan at any offset.
SELECT i.id, i.tenant_id, i.token_epoch, i.status, i.username, i.email,
       i.external_ref, i.assurance_level, i.disabled_at, i.anonymized_at,
       i.created_at, i.last_login_at, i.realm_id, i.category_id,
       r.code AS realm_code, r.kind AS realm_kind,
       c.code AS category_code
FROM identities i
LEFT JOIN realms r ON r.id = i.realm_id
LEFT JOIN realm_categories c ON c.id = i.category_id
WHERE i.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(realm_id)::uuid IS NULL OR i.realm_id = sqlc.narg(realm_id))
  AND (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(query)::text IS NULL
       OR i.username ILIKE '%' || sqlc.narg(query) || '%'
       OR i.email    ILIKE '%' || sqlc.narg(query) || '%')
  AND (sqlc.narg(after_id)::uuid IS NULL OR i.id > sqlc.narg(after_id))
ORDER BY i.id
LIMIT sqlc.arg(page_size);

-- name: CreateIdentity :one
INSERT INTO identities (tenant_id, realm_id, username, email, external_ref,
                        assurance_level, category_id, status)
VALUES (sqlc.arg(tenant_id), sqlc.arg(realm_id), sqlc.arg(username),
        nullif(sqlc.arg(email), ''), nullif(sqlc.arg(external_ref), ''),
        sqlc.arg(assurance_level), sqlc.narg(category_id), sqlc.arg(status))
RETURNING id, created_at, token_epoch;

-- name: DisableIdentity :execrows
UPDATE identities SET status = 'disabled', disabled_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND disabled_at IS NULL;

-- name: EnableIdentity :execrows
UPDATE identities SET status = 'active', disabled_at = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: BumpTokenEpoch :one
UPDATE identities SET token_epoch = token_epoch + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id)
RETURNING token_epoch;

-- name: TouchLastLogin :exec
UPDATE identities SET last_login_at = now() WHERE id = sqlc.arg(id);

-- name: RequestErasure :execrows
UPDATE identities SET deletion_requested_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id)
  AND deletion_requested_at IS NULL;

-- name: LinkIdentities :exec
INSERT INTO identity_links (tenant_id, primary_id, secondary_id, linked_by, method, evidence)
VALUES (sqlc.arg(tenant_id), sqlc.arg(primary_id), sqlc.arg(secondary_id),
        sqlc.arg(linked_by), sqlc.arg(method), sqlc.arg(evidence)::jsonb);

-- name: GetIdentityAuthState :one
-- Introspection / gate: the two identity fields that outrank any token claim.
SELECT token_epoch, status, disabled_at, anonymized_at
FROM identities
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: ExpireRetainedIdentities :many
-- Retention sweep: identities past their statutory limit are anonymised, not
-- deleted — rows and referential integrity survive for audit while
-- authorize() denies from that moment (migrations/0009 gate 1).
UPDATE identities
SET anonymized_at = now(),
    status        = 'disabled',
    username      = 'anon_' || left(replace(id::text, '-', ''), 16),
    email         = NULL,
    external_ref  = NULL,
    attributes    = '{}'::jsonb,
    token_epoch   = token_epoch + 1,
    updated_at    = now()
WHERE retention_until IS NOT NULL
  AND retention_until < now()
  AND anonymized_at IS NULL
RETURNING id, tenant_id, pii_key_id;

-- name: SetRetentionFromRealm :execrows
-- Applies the realm's default_retention to identities that have none yet.
UPDATE identities i
SET retention_until = now() + r.default_retention, updated_at = now()
FROM realms r
WHERE i.realm_id = r.id
  AND r.default_retention IS NOT NULL
  AND i.retention_until IS NULL
  AND i.anonymized_at IS NULL;

-- name: AnonymizeIdentity :one
-- Right-to-erasure execution: crypto-shred the PII key, blank the direct
-- identifiers, bump the epoch so every outstanding token dies.
UPDATE identities
SET anonymized_at = now(),
    status        = 'disabled',
    username      = 'anon_' || left(replace(id::text, '-', ''), 16),
    email         = NULL,
    external_ref  = NULL,
    attributes    = '{}'::jsonb,
    pii_key_id    = NULL,
    token_epoch   = token_epoch + 1,
    updated_at    = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id)
  AND anonymized_at IS NULL
RETURNING id, pii_key_id;
