package repository

import (
	"context"
	"time"
)

type TenantRepository interface {
	TenantBySlug(ctx context.Context, slug string) (*TenantRef, error)
	TenantByID(ctx context.Context, id string) (*TenantRef, error)
	ListTenants(ctx context.Context) ([]TenantRef, error)
	CreateTenant(ctx context.Context, slug, name string) (*TenantRef, error)
	CatalogVersion(ctx context.Context, tenantID string) (int64, time.Time, error)
}
