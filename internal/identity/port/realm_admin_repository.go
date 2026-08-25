package identityport

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

type RealmAdminRepository interface {
	ListRealms(ctx context.Context, tenantID string) ([]identitydomain.RealmRecord, error)
	CreateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) (string, error)
	UpdateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) error
	ListRealmCategories(ctx context.Context, realmID string) ([]identitydomain.RealmCategoryRecord, error)
	// CountIdentitiesByCategory returns category id -> people in it.
	CountIdentitiesByCategory(ctx context.Context, tenantID, realmID string) (map[string]int64, error)
	CreateRealmCategory(ctx context.Context, tenantID string, c identitydomain.RealmCategoryRecord) (string, error)
	RealmCategoryByCode(ctx context.Context, realmID, code string) (*identitydomain.RealmCategoryRecord, error)
}
