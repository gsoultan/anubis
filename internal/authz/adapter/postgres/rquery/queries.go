// Package authzrquery is the ONE place where this context's SQL lives in Go —
// the successor to db/queries/authz/*.sql under ADR-0009 §5: SQL is
// reviewable in exactly one package per context, and every statement here is
// PREPAREd against the live schema at generate time, so a query that drifts
// from the database fails the build naming the column, not the request.
//
// Files mirror the old db/queries/authz layout one-to-one (authz.go, role.go,
// grant.go, membership.go, permission.go) so review diffs read side by side.
//
// uuid columns are selected `::text` and bound as strings, deliberately: the
// domain layer speaks string ids (as it did under sqlc), and the cast keeps
// that contract at the SQL boundary instead of scattering conversions.
package authzrquery

import "github.com/gsoultan/raorm"

// Queries is what cmd/raormgen validates and emits scanners for. Every
// declaration in the package MUST be listed — an omitted one still runs, but
// without generate-time schema checking, which is the property this package
// exists to provide.
func Queries() []raorm.RawDecl {
	return []raorm.RawDecl{
		// authz.go
		Authorize, AuthorizeExplain, GetPermissionByKey, RolesForIdentity,
		EffectivePermissionsForIdentity, AuthorizeStrictSim, SampleAuthorizeDecisions,
		// role.go
		ListRoles, GetRole, CreateRole, UpdateRole,
		ListRoleParents, DeleteRoleParents, InsertRoleParent,
		ListRolePatterns, DeleteRolePatterns, InsertRolePattern,
		DeleteRolePermissions, InsertRolePermission,
		RecomputeRoleEffective, RolesBelow, GetRoleEffective, ListRolesUsingPattern,
		// grant.go
		ListGrantsByIdentity, ListGrantScopes, CreateGrant, InsertGrantScope,
		RevokeGrant, SearchGrants, CountLiveGrants,
		// membership.go
		ListMemberships, GetMembership, CreateMembership,
		ListMembershipEntries, ListMembershipEntryScopes,
		DeleteMembershipEntries, InsertMembershipEntry, InsertMembershipEntryScope,
		AssignMembership, UnassignMembership, ResyncMembership,
		// permission.go
		ListPermissions, UpsertPermission, DeprecatePermissionsExcept, PermissionIDByKey,
	}
}
