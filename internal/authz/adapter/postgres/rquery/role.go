package authzrquery

import (
	"github.com/gsoultan/raorm"
	"github.com/gsoultan/raorm/runtime"
)

// RoleRow is a role with its application slug, the shape ListRoles/GetRole
// return. ApplicationSlug is null when the role is tenant-wide (LEFT JOIN).
type RoleRow struct {
	ID                string
	TenantID          string
	Name              string
	Description       string
	IsSystem          bool
	AllowedRealmKinds []string
	AssignableAt      []string
	ApplicationID     runtime.Null[string]
	ApplicationSlug   runtime.Null[string]
}

// ListRoles is the TENANT's roles: what its own people can be given. Who may
// ADMINISTER the tenant is never here — that is platform_assignments
// (ADR-0011), a different population in different tables. $2 is an optional
// name filter.
var ListRoles = raorm.SQL[RoleRow](`
SELECT r.id::text AS id, r.tenant_id::text AS tenant_id, r.name, r.description,
       r.is_system, r.allowed_realm_kinds, r.assignable_at,
       r.application_id::text AS application_id,
       a.slug AS application_slug
FROM roles r
LEFT JOIN applications a ON a.id = r.application_id
WHERE r.tenant_id = $1
  AND ($2::text IS NULL OR r.name ILIKE '%' || $2 || '%')
ORDER BY r.name`)

// GetRole fetches one role by id within its tenant.
var GetRole = raorm.SQL[RoleRow](`
SELECT r.id::text AS id, r.tenant_id::text AS tenant_id, r.name, r.description,
       r.is_system, r.allowed_realm_kinds, r.assignable_at,
       r.application_id::text AS application_id,
       a.slug AS application_slug
FROM roles r
LEFT JOIN applications a ON a.id = r.application_id
WHERE r.id = $1 AND r.tenant_id = $2`)

// GetRoleByName has no declaration: RoleByName is a generated builder query
// over the rmodel projection (rgen/role), the native half of the migration.

// CreatedRoleRow is a fresh role's id.
type CreatedRoleRow struct {
	ID string
}

// CreateRole mirrors the sqlc statement exactly, nullable application and all.
var CreateRole = raorm.SQL[CreatedRoleRow](`
INSERT INTO roles (tenant_id, name, description, application_id, is_system,
                   allowed_realm_kinds, assignable_at)
VALUES ($1, $2, $3, $4, $5, $6::text[], $7::text[])
RETURNING id::text AS id`)

// UpdateRole edits a non-system role's mutable fields. $1 id, $2 tenant.
var UpdateRole = raorm.SQLExec(`
UPDATE roles
SET description = $3,
    allowed_realm_kinds = $4::text[],
    assignable_at = $5::text[],
    updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND NOT is_system`)

// ParentRow is one direct parent in the composition graph.
type ParentRow struct {
	ParentID string
}

// ListRoleParents lists a role's direct parents.
var ListRoleParents = raorm.SQL[ParentRow](`
SELECT parent_id::text AS parent_id FROM role_parents WHERE role_id = $1`)

// DeleteRoleParents clears a role's parent set before a replace.
var DeleteRoleParents = raorm.SQLExec(`
DELETE FROM role_parents WHERE role_id = $1`)

// InsertRoleParent adds one composition edge.
var InsertRoleParent = raorm.SQLExec(`
INSERT INTO role_parents (role_id, parent_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING`)

// PatternRow is one permission pattern.
type PatternRow struct {
	Pattern string
}

// ListRolePatterns lists a role's permission patterns.
var ListRolePatterns = raorm.SQL[PatternRow](`
SELECT pattern FROM role_permission_patterns WHERE role_id = $1
ORDER BY pattern`)

// DeleteRolePatterns clears a role's pattern set before a replace.
var DeleteRolePatterns = raorm.SQLExec(`
DELETE FROM role_permission_patterns WHERE role_id = $1`)

// InsertRolePattern adds one pattern.
var InsertRolePattern = raorm.SQLExec(`
INSERT INTO role_permission_patterns (role_id, pattern)
VALUES ($1, $2)
ON CONFLICT DO NOTHING`)

// DeleteRolePermissions clears a role's direct permission set.
var DeleteRolePermissions = raorm.SQLExec(`
DELETE FROM role_permissions WHERE role_id = $1`)

// InsertRolePermission adds one direct permission.
var InsertRolePermission = raorm.SQLExec(`
INSERT INTO role_permissions (role_id, permission_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING`)

// DoneRow is a maintenance call's completion marker.
type DoneRow struct {
	Done bool
}

// RecomputeRoleEffective rebuilds one role's effective permission set. The
// function returns void; IS NULL turns that into a scannable boolean, because
// raorm.SQLExec (rightly) refuses statements whose descriptor has columns.
var RecomputeRoleEffective = raorm.SQL[DoneRow](`
SELECT (role_recompute_effective($1) IS NULL) AS done`)

// RoleIDRow is one role id.
type RoleIDRow struct {
	RoleID string
}

// RolesBelow finds descendants in the composition graph: every role that
// inherits (directly or transitively) from this one and therefore needs its
// effective set recomputed.
var RolesBelow = raorm.SQL[RoleIDRow](`
WITH RECURSIVE down AS (
    SELECT rp.role_id FROM role_parents rp WHERE rp.parent_id = $1
    UNION
    SELECT rp.role_id FROM role_parents rp JOIN down d ON rp.parent_id = d.role_id
)
SELECT DISTINCT role_id::text AS role_id FROM down`)

// EffectiveRow is one effective permission and the role that conferred it.
type EffectiveRow struct {
	PermissionKey runtime.Null[string]
	ViaRole       string
}

// GetRoleEffective lists a role's effective permissions with provenance.
var GetRoleEffective = raorm.SQL[EffectiveRow](`
SELECT p.key AS permission_key, vr.name AS via_role
FROM role_permissions_effective rpe
JOIN permissions p ON p.id = rpe.permission_id
JOIN roles vr ON vr.id = rpe.via_role_id
WHERE rpe.role_id = $1
ORDER BY p.key`)

// ListRolesUsingPattern finds every role with a pattern in the tenant: after
// the permission catalog changes, each may match differently and needs
// recomputation.
var ListRolesUsingPattern = raorm.SQL[RoleIDRow](`
SELECT DISTINCT rp.role_id::text AS role_id
FROM role_permission_patterns rp
JOIN roles r ON r.id = rp.role_id
WHERE r.tenant_id = $1`)
