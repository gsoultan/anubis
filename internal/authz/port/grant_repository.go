package authzport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grant"
)

type GrantRepository interface {
	ListGrants(ctx context.Context, tenantID, identityID string, includeRevoked bool) ([]grant.GrantRecord, error)
	GrantScopes(ctx context.Context, grantIDs []string) ([]grant.GrantScopeRecord, error)
	CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error)
	RevokeGrant(ctx context.Context, tenantID, id, reason string) error
}
