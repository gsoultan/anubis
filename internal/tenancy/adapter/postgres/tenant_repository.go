package tenancypg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/gen"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

func (s *Repository) TenantBySlug(ctx context.Context, slug string) (*tenancydomain.TenantRef, error) {
	row, err := s.q(ctx).GetTenantBySlug(ctx, slug)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.TenantRef{ID: row.ID, Slug: row.Slug, Name: row.Name,
		Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *Repository) TenantByID(ctx context.Context, id string) (*tenancydomain.TenantRef, error) {
	row, err := s.q(ctx).GetTenant(ctx, id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.TenantRef{ID: row.ID, Slug: row.Slug, Name: row.Name,
		Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *Repository) ListTenants(ctx context.Context) ([]tenancydomain.TenantRef, error) {
	rows, err := s.q(ctx).ListTenants(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.TenantRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.TenantRef{ID: r.ID, Slug: r.Slug, Name: r.Name,
			Status: r.Status, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Repository) CreateTenant(ctx context.Context, slug, name string) (*tenancydomain.TenantRef, error) {
	row, err := s.q(ctx).CreateTenant(ctx, gen.CreateTenantParams{Slug: slug, Name: name})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.TenantRef{ID: row.ID, Slug: row.Slug, Name: row.Name,
		Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *Repository) CatalogVersion(ctx context.Context, tenantID string) (int64, time.Time, error) {
	row, err := s.q(ctx).GetCatalogVersion(ctx, tenantID)
	if err != nil {
		return 0, time.Time{}, database.MapErr(err)
	}
	return row.Version, row.ChangedAt, nil
}
