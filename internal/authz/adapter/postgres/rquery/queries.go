// Package authzrquery is the ONE file where this context's SQL lives in Go —
// the successor to db/queries/authz/*.sql under ADR-0009's reworded rule: SQL
// is reviewable in exactly one place per context, and every statement here is
// PREPAREd against the live schema at generate time, so a query that drifts
// from the database fails the build naming the column, not the request.
//
// uuid columns are selected `::text` and bound as strings, deliberately: the
// domain layer speaks string ids (as it did under sqlc), and the cast keeps
// that contract at the SQL boundary instead of scattering conversions.
package authzrquery

import "github.com/gsoultan/raorm"

// AuthorizeRow carries the engine's one-bit answer.
type AuthorizeRow struct {
	Allow bool
}

// Authorize is THE engine call. Semantics live in migrations/0013 (+0009
// gates); Go never re-implements them on the online path.
var Authorize = raorm.SQL[AuthorizeRow](`
SELECT authorize($1, $2, $3, $4::jsonb) AS allow`)

// RoleNameRow is one role name.
type RoleNameRow struct {
	Name string
}

// RolesForIdentity lists the distinct live-grant role names for an identity.
var RolesForIdentity = raorm.SQL[RoleNameRow](`
SELECT DISTINCT r.name
FROM grants g
JOIN roles r ON r.id = g.role_id
WHERE g.identity_id = $1 AND g.tenant_id = $2
  AND g.revoked_at IS NULL AND g.valid_from <= now()
  AND (g.valid_until IS NULL OR g.valid_until > now())
ORDER BY r.name`)

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

// Queries is what the generator validates and emits scanners for.
func Queries() []raorm.RawDecl {
	return []raorm.RawDecl{Authorize, RolesForIdentity, CreateRole}
}
