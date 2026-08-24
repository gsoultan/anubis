-- ListPermissions is the TENANT's catalog: what its own applications define.
-- name: ListPermissions :many
SELECT p.id, p.key, p.app_slug, p.resource, p.action, p.risk, p.description,
       p.min_assurance, p.requires_amr, COALESCE(p.max_auth_age::text, '')::text AS max_auth_age,
       p.deprecated_at
FROM permissions p
WHERE p.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(application_id)::uuid IS NULL
       OR p.application_id = sqlc.narg(application_id))
  AND (sqlc.arg(include_deprecated)::boolean OR p.deprecated_at IS NULL)
ORDER BY p.key;

-- name: UpsertPermission :one
-- Manifest apply: reactivates a previously deprecated permission rather than
-- duplicating it — never orphan a live grant.
INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action,
                         description, risk, min_assurance, requires_amr, max_auth_age)
VALUES (sqlc.arg(application_id), sqlc.arg(tenant_id), sqlc.arg(app_slug),
        sqlc.arg(resource), sqlc.arg(action), sqlc.arg(description),
        sqlc.arg(risk), sqlc.arg(min_assurance),
        sqlc.arg(requires_amr)::text[],
        nullif(sqlc.arg(max_auth_age), '')::interval)
ON CONFLICT (application_id, resource, action) DO UPDATE
    SET description = EXCLUDED.description,
        risk = EXCLUDED.risk,
        min_assurance = EXCLUDED.min_assurance,
        requires_amr = EXCLUDED.requires_amr,
        max_auth_age = EXCLUDED.max_auth_age,
        deprecated_at = NULL
RETURNING id, key, (xmax = 0) AS inserted;

-- name: DeprecatePermissionsExcept :many
-- Manifest apply: everything the new manifest no longer names is deprecated,
-- never deleted.
UPDATE permissions
SET deprecated_at = now()
WHERE application_id = sqlc.arg(application_id)
  AND deprecated_at IS NULL
  AND NOT (id = ANY(sqlc.arg(keep_ids)::uuid[]))
RETURNING key;

-- name: PermissionIDByKey :one
SELECT id FROM permissions
WHERE tenant_id = sqlc.arg(tenant_id) AND key = sqlc.arg(key)
  AND deprecated_at IS NULL;
