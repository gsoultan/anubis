package identityapp

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
)

// IdentityLifecycleUsecase is directory administration for identities.
type IdentityLifecycleUsecase interface {
	ListIdentities(ctx context.Context, f identitydomain.IdentityFilter) ([]identitydomain.IdentityRecord, string, error)
	GetIdentity(ctx context.Context, id string) (*identitydomain.IdentityRecord, []credential.CredentialInfo, error)
	CreateIdentity(ctx context.Context, in AdminCreateIdentity) (*identitydomain.IdentityRecord, error)
	DisableIdentity(ctx context.Context, id, reason string) error
	EnableIdentity(ctx context.Context, id string) error
	BumpTokenEpoch(ctx context.Context, id string) (int, error)
	SetPassword(ctx context.Context, id, password string) error
	LinkIdentities(ctx context.Context, primaryID, secondaryID, method, evidenceJSON string) error
	RequestErasure(ctx context.Context, id string) error
}
