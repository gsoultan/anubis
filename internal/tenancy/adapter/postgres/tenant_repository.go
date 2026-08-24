package tenancypg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
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

// UpdateTenant renames a tenant. The slug is deliberately not editable: it
// appears in URLs, tokens and every hosted page path, and changing it would
// break links that already exist in the world.
func (s *Repository) UpdateTenant(ctx context.Context, id, name string) error {
	n, err := s.q(ctx).UpdateTenant(ctx, gen.UpdateTenantParams{ID: id, Name: name})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("tenant", id)
	}
	return nil
}

// SetTenantStatus suspends or retires a tenant.
func (s *Repository) SetTenantStatus(ctx context.Context, id, status string) error {
	n, err := s.q(ctx).SetTenantStatus(ctx, gen.SetTenantStatusParams{ID: id, Status: status})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("tenant", id)
	}
	return nil
}

// CountTenantIdentities backs the "this holds N people" warning before a
// tenant is retired.
func (s *Repository) CountTenantIdentities(ctx context.Context, tenantID string) (int, error) {
	n, err := s.q(ctx).CountTenantIdentities(ctx, tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(n), nil
}

// TenantStats counts what a tenant holds.
func (s *Repository) TenantStats(ctx context.Context, tenantID string) (*tenancydomain.TenantStats, error) {
	row, err := s.q(ctx).GetTenantStats(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.TenantStats{
		Identities:  int(row.Identities),
		Grants:      int(row.Grants),
		ScopeNodes:  int(row.ScopeNodes),
		Memberships: int(row.Memberships),
	}, nil
}
