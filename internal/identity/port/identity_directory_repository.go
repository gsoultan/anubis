package identityport

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

type IdentityDirectoryRepository interface {
	ListIdentities(ctx context.Context, tenantID string, f identitydomain.IdentityFilter) ([]identitydomain.IdentityRecord, error)
	IdentityRecordByID(ctx context.Context, tenantID, id string) (*identitydomain.IdentityRecord, error)
}
