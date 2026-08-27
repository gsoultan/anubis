package identityport

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

type RealmAdminRepository interface {
	ListRealms(ctx context.Context, tenantID string) ([]identitydomain.RealmRecord, error)
	CreateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) (string, error)
	// UpdateRealm changes policy. It deliberately cannot change `code` or
	// `kind`: kind decides which roles a realm's members may hold, and
	// migrations/0010 enforces that on every grant.
	UpdateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) error
	// CorrectEmptyRealmIdentity fixes a code or kind typed wrongly at
	// creation, and only while the realm has no members — a realm that has
	// admitted nobody has decided nothing. Reports false when it has members.
	CorrectEmptyRealmIdentity(ctx context.Context, tenantID, realmID, code, kind string) (bool, error)
	ListRealmCategories(ctx context.Context, realmID string) ([]identitydomain.RealmCategoryRecord, error)
	// CountIdentitiesByCategory returns category id -> people in it.
	CountIdentitiesByCategory(ctx context.Context, tenantID, realmID string) (map[string]int64, error)
	CreateRealmCategory(ctx context.Context, tenantID string, c identitydomain.RealmCategoryRecord) (string, error)
	RealmCategoryByCode(ctx context.Context, realmID, code string) (*identitydomain.RealmCategoryRecord, error)
}
