package repository

import "context"

// CredentialRepository stores authentication factors.
type CredentialRepository interface {
	PasswordCredential(ctx context.Context, identityID string) (*Credential, error)
	CreateCredential(ctx context.Context, in CredentialInput) (string, error)
	RevokeCredential(ctx context.Context, tenantID, id string) error
	RevokeCredentialsOfKind(ctx context.Context, identityID, kind string) (int64, error)
	UpdateCredentialSecret(ctx context.Context, id, secret string) error
	UpdateCredentialParams(ctx context.Context, id string, params []byte) error
	ActiveCredentialOfKind(ctx context.Context, identityID, kind string) (*Credential, error)
	ActiveCredentialKinds(ctx context.Context, identityID string) ([]string, error)
	CredentialByLookup(ctx context.Context, lookupKey string) (*APIKeyCredential, error)
	TouchCredentialUsed(ctx context.Context, id string, signCounter int64)
	ListCredentials(ctx context.Context, identityID, kind string) ([]CredentialInfo, error)
	CredentialOwner(ctx context.Context, id string) (identityID string, tenantID string, kind string, err error)
}
