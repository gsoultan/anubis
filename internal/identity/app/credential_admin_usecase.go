package identityapp

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/domain/credential"
)

// CredentialAdminUsecase manages factors and API keys.
type CredentialAdminUsecase interface {
	ListCredentials(ctx context.Context, identityID string) ([]credential.CredentialInfo, error)
	RevokeCredential(ctx context.Context, credentialID string) error
}
