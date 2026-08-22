package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) TenantBySlug(ctx context.Context, slug string) (*repository.TenantRef, error) {
	row, err := s.q(ctx).GetTenantBySlug(ctx, slug)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.TenantRef{ID: row.ID, Slug: row.Slug, Name: row.Name,
		Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *Store) TenantByID(ctx context.Context, id string) (*repository.TenantRef, error) {
	row, err := s.q(ctx).GetTenant(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.TenantRef{ID: row.ID, Slug: row.Slug, Name: row.Name,
		Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *Store) ListTenants(ctx context.Context) ([]repository.TenantRef, error) {
	rows, err := s.q(ctx).ListTenants(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.TenantRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.TenantRef{ID: r.ID, Slug: r.Slug, Name: r.Name,
			Status: r.Status, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Store) CreateTenant(ctx context.Context, slug, name string) (*repository.TenantRef, error) {
	row, err := s.q(ctx).CreateTenant(ctx, gen.CreateTenantParams{Slug: slug, Name: name})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.TenantRef{ID: row.ID, Slug: row.Slug, Name: row.Name,
		Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *Store) CatalogVersion(ctx context.Context, tenantID string) (int64, time.Time, error) {
	row, err := s.q(ctx).GetCatalogVersion(ctx, tenantID)
	if err != nil {
		return 0, time.Time{}, mapErr(err)
	}
	return row.Version, row.ChangedAt, nil
}
