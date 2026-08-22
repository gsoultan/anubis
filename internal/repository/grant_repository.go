package repository

import "context"

type GrantRepository interface {
	ListGrants(ctx context.Context, tenantID, identityID string, includeRevoked bool) ([]GrantRecord, error)
	GrantScopes(ctx context.Context, grantIDs []string) ([]GrantScopeRecord, error)
	CreateGrant(ctx context.Context, in GrantCreate) (string, error)
	RevokeGrant(ctx context.Context, tenantID, id, reason string) error
}
