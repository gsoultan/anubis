package identityport

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

// RealmRepository resolves population policy.
type RealmRepository interface {
	RealmByCode(ctx context.Context, tenantID, code string) (*identitydomain.Realm, error)
	RealmByID(ctx context.Context, id string) (*identitydomain.Realm, error)
}
