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
	// UpdateCredentialSecret writes the secret and the local key it was
	// sealed under. kid is empty for a password, which is hashed rather than
	// sealed; it is REQUIRED for anything the keyring seals, because a
	// secret read for the life of an enrolment cannot depend on which key
	// happens to be active when it is read.
	UpdateCredentialSecret(ctx context.Context, id, secret, kid string) error
	UpdateCredentialParams(ctx context.Context, id string, params []byte) error
	// AdvanceCredentialStep records an accepted TOTP step and reports
	// whether this caller recorded it. false is a REPLAY: somebody already
	// used this code. The comparison belongs to the database because a
	// read-then-write in Go lets every concurrent presentation pass.
	AdvanceCredentialStep(ctx context.Context, id string, step uint64) (bool, error)
	ActiveCredentialOfKind(ctx context.Context, identityID, kind string) (*credential.Credential, error)
	ActiveCredentialKinds(ctx context.Context, identityID string) ([]string, error)
	TouchCredentialUsed(ctx context.Context, id string, signCounter int64)
	// ListCredentials is tenant-scoped: identityID reaches this from an admin
	// request, and an identity id alone proves nothing about who may read it.
	ListCredentials(ctx context.Context, tenantID, identityID, kind string) ([]credential.CredentialInfo, error)
	CredentialOwner(ctx context.Context, id string) (identityID string, tenantID string, kind string, err error)
}
