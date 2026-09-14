package authzport

import (
	"context"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
)

// RoleCatalogRepository is the manifest's side of a role, kept apart from
// RoleRepository for the same reason PermissionCatalogRepository is: applying
// a catalog is a different job from administering one. An operator edits a
// role they made; a document installs and retires the roles it owns, and the
// two must not reach for each other's writes.
type RoleCatalogRepository interface {
	// UpsertSystemRole installs or refreshes a role the manifest declares,
	// clearing any deprecation. It refuses a name already held by a role the
	// manifest does not own.
	UpsertSystemRole(ctx context.Context, tenantID, applicationID string, r authzdomain.RoleRecord) (string, error)
	// DeprecateRolesExcept retires the manifest roles the document stopped
	// naming, returning their names. Existing grants keep working.
	DeprecateRolesExcept(ctx context.Context, applicationID string, keepIDs []string) ([]string, error)
}
