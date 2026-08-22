package tenancyport

import (
	"context"
	"time"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

type TenantRepository interface {
	TenantBySlug(ctx context.Context, slug string) (*tenancydomain.TenantRef, error)
	TenantByID(ctx context.Context, id string) (*tenancydomain.TenantRef, error)
	ListTenants(ctx context.Context) ([]tenancydomain.TenantRef, error)
	CreateTenant(ctx context.Context, slug, name string) (*tenancydomain.TenantRef, error)
	CatalogVersion(ctx context.Context, tenantID string) (int64, time.Time, error)
}
