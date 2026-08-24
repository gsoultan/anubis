package provisioningport

import (
	"context"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

// AccessReader resolves the names a workbook grants access by into ids.
type AccessReader interface {
	RoleByName(ctx context.Context, tenantID, name string) (*authzdomain.RoleRecord, error)
	ListMemberships(ctx context.Context, tenantID string) ([]membership.MembershipRecord, error)
	// ListGrants is what makes re-running an import a no-op rather than a
	// second pile of duplicate grants.
	ListGrants(ctx context.Context, tenantID, identityID string, includeRevoked bool) ([]grant.GrantRecord, error)
}
