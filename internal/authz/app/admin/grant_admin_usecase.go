package authzadmin

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grant"
)

type GrantAdminUsecase interface {
	ListGrants(ctx context.Context, identityID string, includeRevoked bool) ([]grant.GrantRecord, []grant.GrantScopeRecord, error)
	// SearchGrants backs the Access screen. There is no "list every grant":
	// a tenant can hold hundreds of thousands, and that is not a question
	// any screen can answer usefully.
	SearchGrants(ctx context.Context, q grant.GrantSearch) ([]grant.GrantHit, []grant.GrantScopeRecord, error)
	CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error)
	RevokeGrant(ctx context.Context, grantID, reason string) error
}
