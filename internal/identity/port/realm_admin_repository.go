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
	// ListRealmCategories is tenant-scoped; an empty realmID means every
	// realm the tenant owns. Both matter: the tenant filter is what keeps one
	// tenant's realm id from reading another's categories, and "all realms"
	// is what a screen spanning populations actually asks for.
	ListRealmCategories(ctx context.Context, tenantID, realmID string) ([]identitydomain.RealmCategoryRecord, error)
	// CountIdentitiesByCategory returns category id -> people in it, within
	// one realm. An empty realmID is accepted and counts across the tenant,
	// but that is a grouped scan of the whole directory -- the RPC only asks
	// when a realm was named.
	CountIdentitiesByCategory(ctx context.Context, tenantID, realmID string) (map[string]int64, error)
	CreateRealmCategory(ctx context.Context, tenantID string, c identitydomain.RealmCategoryRecord) (string, error)
	RealmCategoryByCode(ctx context.Context, realmID, code string) (*identitydomain.RealmCategoryRecord, error)
}
