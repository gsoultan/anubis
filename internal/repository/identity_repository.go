package repository

import (
	"context"
	"github.com/gsoultan/anubis/internal/domain"
)

// IdentityRepository is identity lifecycle persistence.
type IdentityRepository interface {
	IdentityForLogin(ctx context.Context, tenantID, realmID, username string) (*domain.Identity, error)
	Identity(ctx context.Context, tenantID, id string) (*domain.Identity, error)
	CreateIdentity(ctx context.Context, in IdentityCreate) (string, error)
	DisableIdentity(ctx context.Context, tenantID, id string) error
	EnableIdentity(ctx context.Context, tenantID, id string) error
	BumpTokenEpoch(ctx context.Context, tenantID, id string) (int, error)
	TouchLastLogin(ctx context.Context, id string)
	RequestErasure(ctx context.Context, tenantID, id string) error
	LinkIdentities(ctx context.Context, tenantID, primaryID, secondaryID, linkedBy, method string, evidence []byte) error
}
