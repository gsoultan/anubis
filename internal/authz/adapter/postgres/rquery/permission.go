package authzrquery

import (
	"time"

	"github.com/gsoultan/raorm"
	"github.com/gsoultan/raorm/runtime"
)

// PermissionRow is one catalog permission, interval rendered as text the way
// the sqlc query did.
type PermissionRow struct {
	ID           string
	Key          runtime.Null[string]
	AppSlug      string
	Resource     string
	Action       string
	Risk         string
	Description  string
	MinAssurance int16
	RequiresAmr  []string
	MaxAuthAge   string
	DeprecatedAt runtime.Null[time.Time]
}

// ListPermissions is the TENANT's catalog: what its own applications define.
// $2 optional application filter, $3 include_deprecated.
var ListPermissions = raorm.SQL[PermissionRow](`
SELECT p.id::text AS id, p.key, p.app_slug, p.resource, p.action, p.risk,
       p.description, p.min_assurance, p.requires_amr,
       COALESCE(p.max_auth_age::text, '')::text AS max_auth_age,
       p.deprecated_at
FROM permissions p
WHERE p.tenant_id = $1
  AND ($2::uuid IS NULL OR p.application_id = $2)
  AND ($3::boolean OR p.deprecated_at IS NULL)
ORDER BY p.key`)

// UpsertedPermissionRow reports the row and whether it was newly inserted.
type UpsertedPermissionRow struct {
	ID       string
	Key      runtime.Null[string]
	Inserted runtime.Null[bool]
}

// UpsertPermission is manifest apply: it reactivates a previously deprecated
// permission rather than duplicating it — never orphan a live grant.
// $10 max_auth_age as interval text (” becomes NULL).
var UpsertPermission = raorm.SQL[UpsertedPermissionRow](`
INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action,
                         description, risk, min_assurance, requires_amr, max_auth_age)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::text[], nullif($10, '')::interval)
ON CONFLICT (application_id, resource, action) DO UPDATE
    SET description = EXCLUDED.description,
        risk = EXCLUDED.risk,
        min_assurance = EXCLUDED.min_assurance,
        requires_amr = EXCLUDED.requires_amr,
        max_auth_age = EXCLUDED.max_auth_age,
        deprecated_at = NULL
RETURNING id::text AS id, key, (xmax = 0) AS inserted`)

// DeprecatedKeyRow is one deprecated permission's key.
type DeprecatedKeyRow struct {
	Key runtime.Null[string]
}

// DeprecatePermissionsExcept is manifest apply's other half: everything the
// new manifest no longer names is deprecated, never deleted.
var DeprecatePermissionsExcept = raorm.SQL[DeprecatedKeyRow](`
UPDATE permissions
SET deprecated_at = now()
WHERE application_id = $1
  AND deprecated_at IS NULL
  AND NOT (id = ANY($2::uuid[]))
RETURNING key`)

// PermissionIDRow is one permission's id.
type PermissionIDRow struct {
	ID string
}

// PermissionIDByKey resolves a live permission key to its id.
var PermissionIDByKey = raorm.SQL[PermissionIDRow](`
SELECT id::text AS id FROM permissions
WHERE tenant_id = $1 AND key = $2
  AND deprecated_at IS NULL`)
