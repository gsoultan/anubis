package usecase

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/repository"
)

// TenantOpsUsecase, RealmAdminUsecase etc. are grouped here as one composed
// interface per plane; each stays within the 15-method budget.

type TenantOpsUsecase interface {
	ListTenants(ctx context.Context) ([]repository.TenantRef, error)
	CreateTenant(ctx context.Context, slug, name string) (*repository.TenantRef, error)
}

type RealmAdminUsecase interface {
	ListRealms(ctx context.Context) ([]repository.RealmRecord, error)
	CreateRealm(ctx context.Context, r repository.RealmRecord) (*repository.RealmRecord, error)
	UpdateRealm(ctx context.Context, r repository.RealmRecord) (*repository.RealmRecord, error)
	ListRealmCategories(ctx context.Context, realmID string) ([]repository.RealmCategoryRecord, error)
	CreateRealmCategory(ctx context.Context, c repository.RealmCategoryRecord) (*repository.RealmCategoryRecord, error)
}

type ApplicationAdminUsecase interface {
	ListApplications(ctx context.Context) ([]repository.ApplicationRecord, error)
	CreateApplication(ctx context.Context, a repository.ApplicationRecord) (*repository.ApplicationRecord, string, error)
	UpdateApplication(ctx context.Context, a repository.ApplicationRecord) (*repository.ApplicationRecord, error)
	RotateClientSecret(ctx context.Context, applicationID string) (string, error)
	ListRoutePolicies(ctx context.Context, applicationSlug string) ([]repository.RoutePolicyRecord, error)
}

type AuditAdminUsecase interface {
	QueryAudit(ctx context.Context, q repository.AuditQuery) ([]repository.AuditRecord, string, error)
	VerifyAuditChain(ctx context.Context, from, to *time.Time) (checked, brokenAt int64, err error)
}

type KeyAdminUsecase interface {
	ListSigningKeys(ctx context.Context) ([]repository.KeyRecord, error)
	RotateSigningKey(ctx context.Context, purpose string) (*repository.KeyRecord, error)
}

type SiteAdminUsecase interface {
	GetCatalogVersion(ctx context.Context) (int64, time.Time, error)
	GetSigninPage(ctx context.Context) ([]byte, time.Time, error)
	PutSigninPage(ctx context.Context, config []byte) error
}

// TenantAdminUsecase composes tenant-level administration
// (proto TenantAdminService).
type TenantAdminUsecase interface {
	TenantOpsUsecase
	RealmAdminUsecase
	ApplicationAdminUsecase
	AuditAdminUsecase
	KeyAdminUsecase
	SiteAdminUsecase
}
