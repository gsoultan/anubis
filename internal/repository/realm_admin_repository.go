package repository

import "context"

type RealmAdminRepository interface {
	ListRealms(ctx context.Context, tenantID string) ([]RealmRecord, error)
	CreateRealm(ctx context.Context, tenantID string, r RealmRecord) (string, error)
	UpdateRealm(ctx context.Context, tenantID string, r RealmRecord) error
	ListRealmCategories(ctx context.Context, realmID string) ([]RealmCategoryRecord, error)
	CreateRealmCategory(ctx context.Context, tenantID string, c RealmCategoryRecord) (string, error)
	RealmCategoryByCode(ctx context.Context, realmID, code string) (*RealmCategoryRecord, error)
}
