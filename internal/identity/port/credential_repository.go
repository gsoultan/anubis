package identityport

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/domain/credential"
)

// CredentialRepository stores authentication factors.
type CredentialRepository interface {
	PasswordCredential(ctx context.Context, identityID string) (*credential.Credential, error)
	CreateCredential(ctx context.Context, in credential.CredentialInput) (string, error)
	RevokeCredential(ctx context.Context, tenantID, id string) error
	RevokeCredentialsOfKind(ctx context.Context, identityID, kind string) (int64, error)
	UpdateCredentialSecret(ctx context.Context, id, secret string) error
	UpdateCredentialParams(ctx context.Context, id string, params []byte) error
	ActiveCredentialOfKind(ctx context.Context, identityID, kind string) (*credential.Credential, error)
	ActiveCredentialKinds(ctx context.Context, identityID string) ([]string, error)
	CredentialByLookup(ctx context.Context, lookupKey string) (*credential.APIKeyCredential, error)
	TouchCredentialUsed(ctx context.Context, id string, signCounter int64)
	ListCredentials(ctx context.Context, identityID, kind string) ([]credential.CredentialInfo, error)
	CredentialOwner(ctx context.Context, id string) (identityID string, tenantID string, kind string, err error)
}
