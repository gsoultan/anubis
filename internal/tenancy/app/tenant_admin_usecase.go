package tenancyapp

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// TenantOpsUsecase, RealmAdminUsecase etc. are grouped here as one composed
// interface per plane; each stays within the 15-method budget.

type TenantOpsUsecase interface {
	ListTenants(ctx context.Context) ([]tenancydomain.TenantRef, error)
	CreateTenant(ctx context.Context, slug, name string) (*tenancydomain.TenantRef, error)
}

type RealmAdminUsecase interface {
	ListRealms(ctx context.Context) ([]identitydomain.RealmRecord, error)
	CreateRealm(ctx context.Context, r identitydomain.RealmRecord) (*identitydomain.RealmRecord, error)
	UpdateRealm(ctx context.Context, r identitydomain.RealmRecord) (*identitydomain.RealmRecord, error)
	ListRealmCategories(ctx context.Context, realmID string) ([]identitydomain.RealmCategoryRecord, error)
	CreateRealmCategory(ctx context.Context, c identitydomain.RealmCategoryRecord) (*identitydomain.RealmCategoryRecord, error)
}

type ApplicationAdminUsecase interface {
	ListApplications(ctx context.Context) ([]tenancydomain.ApplicationRecord, error)
	CreateApplication(ctx context.Context, a tenancydomain.ApplicationRecord) (*tenancydomain.ApplicationRecord, string, error)
	UpdateApplication(ctx context.Context, a tenancydomain.ApplicationRecord) (*tenancydomain.ApplicationRecord, error)
	RotateClientSecret(ctx context.Context, applicationID string) (string, error)
	ListRoutePolicies(ctx context.Context, applicationSlug string) ([]tenancydomain.RoutePolicyRecord, error)
}

type AuditAdminUsecase interface {
	QueryAudit(ctx context.Context, q auditdomain.AuditQuery) ([]auditdomain.AuditRecord, string, error)
	VerifyAuditChain(ctx context.Context, from, to *time.Time) (checked, brokenAt int64, err error)
}

type KeyAdminUsecase interface {
	ListSigningKeys(ctx context.Context) ([]authdomain.KeyRecord, error)
	RotateSigningKey(ctx context.Context, purpose string) (*authdomain.KeyRecord, error)
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
