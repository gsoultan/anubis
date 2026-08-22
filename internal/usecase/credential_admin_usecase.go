package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

// CredentialAdminUsecase manages factors and API keys.
type CredentialAdminUsecase interface {
	ListCredentials(ctx context.Context, identityID string) ([]repository.CredentialInfo, error)
	RevokeCredential(ctx context.Context, credentialID string) error
	// CreateAPIKey returns the FULL key exactly once; only prefix+hash persist.
	CreateAPIKey(ctx context.Context, identityID, label string, expiresAt int64) (apiKey, prefix, credentialID string, err error)
	ListAPIKeys(ctx context.Context, identityID string) ([]repository.CredentialInfo, error)
}
