package repository

import "context"

// AuthzRepository fronts the SQL decision engine. Semantics live in
// migrations/0013; Go never re-implements them on the online path.
type AuthzRepository interface {
	Authorize(ctx context.Context, identityID, tenantID, permission string, targets []byte) (bool, error)
	AuthorizeExplain(ctx context.Context, identityID, tenantID, permission string, targets []byte) (string, error)
	AuthorizeStrictSim(ctx context.Context, identityID, tenantID, permission string, targets []byte, strictAxis string) (bool, error)
	PermissionByKey(ctx context.Context, tenantID, key string) (*PermissionMeta, error)
	RolesForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error)
	EffectivePermissionsForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error)
	SampleAuthorizeDecisions(ctx context.Context, tenantID string, n int) ([][]byte, error)
}
