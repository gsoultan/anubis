package authzadmin

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grant"
)

type GrantAdminUsecase interface {
	ListGrants(ctx context.Context, identityID string, includeRevoked bool) ([]grant.GrantRecord, []grant.GrantScopeRecord, error)
	CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error)
	RevokeGrant(ctx context.Context, grantID, reason string) error
}
