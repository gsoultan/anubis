-- ListRoles is the TENANT's roles: what its own people can be given. Who may
-- ADMINISTER the tenant is never here — that is platform_assignments
-- (ADR-0011), a different population in different tables.
-- name: ListRoles :many
SELECT r.id, r.tenant_id, r.name, r.description, r.is_system,
       r.allowed_realm_kinds, r.assignable_at, r.application_id,
       a.slug AS application_slug
FROM roles r
LEFT JOIN applications a ON a.id = r.application_id
WHERE r.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(query)::text IS NULL OR r.name ILIKE '%' || sqlc.narg(query) || '%')
ORDER BY r.name;

-- name: GetRole :one
SELECT r.id, r.tenant_id, r.name, r.description, r.is_system,
       r.allowed_realm_kinds, r.assignable_at, r.application_id,
       a.slug AS application_slug
FROM roles r
LEFT JOIN applications a ON a.id = r.application_id
WHERE r.id = sqlc.arg(id) AND r.tenant_id = sqlc.arg(tenant_id);

-- name: GetRoleByName :one
SELECT id, tenant_id, name, description, is_system, allowed_realm_kinds
FROM roles
WHERE tenant_id = sqlc.arg(tenant_id) AND name = sqlc.arg(name);

-- name: CreateRole :one
INSERT INTO roles (tenant_id, name, description, application_id, is_system,
                   allowed_realm_kinds, assignable_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description),
        sqlc.narg(application_id), sqlc.arg(is_system),
        sqlc.arg(allowed_realm_kinds)::text[], sqlc.arg(assignable_at)::text[])
RETURNING id;

-- name: UpdateRole :execrows
UPDATE roles
SET description = sqlc.arg(description),
    allowed_realm_kinds = sqlc.arg(allowed_realm_kinds)::text[],
    assignable_at = sqlc.arg(assignable_at)::text[],
    updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND NOT is_system;

-- name: ListRoleParents :many
SELECT parent_id FROM role_parents WHERE role_id = sqlc.arg(role_id);

-- name: DeleteRoleParents :exec
DELETE FROM role_parents WHERE role_id = sqlc.arg(role_id);

-- name: InsertRoleParent :exec
INSERT INTO role_parents (role_id, parent_id)
VALUES (sqlc.arg(role_id), sqlc.arg(parent_id))
ON CONFLICT DO NOTHING;

-- name: ListRolePatterns :many
SELECT pattern FROM role_permission_patterns WHERE role_id = sqlc.arg(role_id)
ORDER BY pattern;

-- name: DeleteRolePatterns :exec
DELETE FROM role_permission_patterns WHERE role_id = sqlc.arg(role_id);

-- name: InsertRolePattern :exec
INSERT INTO role_permission_patterns (role_id, pattern)
VALUES (sqlc.arg(role_id), sqlc.arg(pattern))
ON CONFLICT DO NOTHING;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = sqlc.arg(role_id);

-- name: InsertRolePermission :exec
INSERT INTO role_permissions (role_id, permission_id)
VALUES (sqlc.arg(role_id), sqlc.arg(permission_id))
ON CONFLICT DO NOTHING;

-- name: RecomputeRoleEffective :exec
SELECT role_recompute_effective(sqlc.arg(role_id));

-- name: RolesBelow :many
-- Descendants in the composition graph: every role that inherits (directly or
-- transitively) from this one and therefore needs its effective set recomputed.
WITH RECURSIVE down AS (
    SELECT rp.role_id FROM role_parents rp WHERE rp.parent_id = sqlc.arg(role_id)
    UNION
    SELECT rp.role_id FROM role_parents rp JOIN down d ON rp.parent_id = d.role_id
)
SELECT DISTINCT role_id FROM down;

-- name: GetRoleEffective :many
SELECT p.key AS permission_key, vr.name AS via_role
FROM role_permissions_effective rpe
JOIN permissions p ON p.id = rpe.permission_id
JOIN roles vr ON vr.id = rpe.via_role_id
WHERE rpe.role_id = sqlc.arg(role_id)
ORDER BY p.key;

-- name: ListRolesUsingPattern :many
-- After the permission catalog changes, every role with a pattern may match
-- differently and needs recomputation.
SELECT DISTINCT rp.role_id
FROM role_permission_patterns rp
JOIN roles r ON r.id = rp.role_id
WHERE r.tenant_id = sqlc.arg(tenant_id);
