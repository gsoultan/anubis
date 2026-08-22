package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

// IdentityLifecycleUsecase is directory administration for identities.
type IdentityLifecycleUsecase interface {
	ListIdentities(ctx context.Context, f repository.IdentityFilter) ([]repository.IdentityRecord, string, error)
	GetIdentity(ctx context.Context, id string) (*repository.IdentityRecord, []repository.CredentialInfo, error)
	CreateIdentity(ctx context.Context, in AdminCreateIdentity) (*repository.IdentityRecord, error)
	DisableIdentity(ctx context.Context, id, reason string) error
	EnableIdentity(ctx context.Context, id string) error
	BumpTokenEpoch(ctx context.Context, id string) (int, error)
	SetPassword(ctx context.Context, id, password string) error
	LinkIdentities(ctx context.Context, primaryID, secondaryID, method, evidenceJSON string) error
	RequestErasure(ctx context.Context, id string) error
}
