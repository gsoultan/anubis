package identityapp

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/identity/domain/credential"
)

// CredentialAdminUsecase manages factors and API keys.
type CredentialAdminUsecase interface {
	ListCredentials(ctx context.Context, identityID string) ([]credential.CredentialInfo, error)
	RevokeCredential(ctx context.Context, credentialID string) error
	// ImportPasswordHash installs a password hash made by another system as
	// the identity's password, replacing any it holds. The hash must be one
	// kdf.Verify understands; a legacy format MUST carry a future expiresAt,
	// after which it stops working and the person needs a reset. The person
	// signs in with the password they already know and the hash is replaced
	// by one at this installation's cost on that first sign-in.
	ImportPasswordHash(ctx context.Context, identityID, hash string, expiresAt time.Time) error
}
