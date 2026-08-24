package provisioningport

import (
	"context"

	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

// PeopleWriter creates identities.
//
// It is deliberately the identity context's own admin usecase rather than
// its repository: password policy, realm and category validation, the
// permission check and the audit event all live in that usecase, and an
// importer that wrote rows straight to the table would skip every one of
// them.
type PeopleWriter interface {
	CreateIdentity(ctx context.Context, in identityapp.AdminCreateIdentity) (*identitydomain.IdentityRecord, error)
}
