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

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed, and an omitted one does not
// merely lose its generate-time schema check: as of storm v0.10.0 it REFUSES
// TO RUN, at runtime, with "this statement was not declared at generate
// time". The build is clean and the tests pass — a scheduled job is where you
// find out. Adding a declaration is two edits, here and `storm generate`.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		// authz.go
		Authorize, AuthorizeExplain, GetPermissionByKey, RolesForIdentity,
		EffectiveGrantsForIdentity, ScopeForestForTenant,
		EffectivePermissionsForIdentity, AuthorizeStrictSim, SampleAuthorizeDecisions,
		// role.go
		ListRoles, GetRole, CreateRole, UpdateRole,
		ListRoleParents, DeleteRoleParents, InsertRoleParent,
		ListRolePatterns, DeleteRolePatterns, InsertRolePattern,
		DeleteRolePermissions, InsertRolePermission,
		RecomputeRoleEffective, RolesBelow, GetRoleEffective, ListRolesUsingPattern,
		UpsertSystemRole, DeprecateRolesExcept,
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
		// catalog_sync.go
		ListCatalogSources, GetCatalogSource, DueCatalogSources,
		CreateCatalogSource, UpdateCatalogSource, DeleteCatalogSource, ScheduleCatalogSource,
		StartCatalogRun, FinishCatalogRun, ListCatalogRuns, LastAppliedDigest,
	}
}
