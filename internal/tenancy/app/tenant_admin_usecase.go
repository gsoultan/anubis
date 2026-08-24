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
	UpdateTenant(ctx context.Context, id, name string) error
	// SetTenantStatus is how a tenant is suspended or retired; "archived" is
	// what deleting one means, because the row is what every record in the
	// installation hangs off.
	SetTenantStatus(ctx context.Context, id, status string) error
	TenantStats(ctx context.Context, id string) (*tenancydomain.TenantStats, error)
	// API keys are the TENANT's machine credentials: they authenticate as
	// the tenant's system, never as any person (migration 0030).
	ListAPIKeys(ctx context.Context) ([]authdomain.APIKeyRecord, error)
	CreateAPIKey(ctx context.Context, label string, expiresAt int64) (fullKey, prefix, id string, err error)
	RevokeAPIKey(ctx context.Context, id string) error
}

type RealmAdminUsecase interface {
	ListRealms(ctx context.Context) ([]identitydomain.RealmRecord, error)
	CreateRealm(ctx context.Context, r identitydomain.RealmRecord) (*identitydomain.RealmRecord, error)
	UpdateRealm(ctx context.Context, r identitydomain.RealmRecord) (*identitydomain.RealmRecord, error)
	ListRealmCategories(ctx context.Context, realmID string) ([]identitydomain.RealmCategoryRecord, error)
	CreateRealmCategory(ctx context.Context, c identitydomain.RealmCategoryRecord) (*identitydomain.RealmCategoryRecord, error)
}

type ApplicationAdminUsecase interface {
	// ListApplications is one page of the TENANT's relying parties. Anubis's
	// own applications are not among them: they own the permission catalog
	// and are nothing a tenant's people sign in to.
	ListApplications(ctx context.Context, query, cursor string, pageSize int) ([]tenancydomain.ApplicationRecord, int, error)
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
