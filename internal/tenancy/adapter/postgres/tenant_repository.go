package tenancypg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rgen/catalogversion"
	"github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rgen/tenant"
	tenancyrquery "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rquery"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

func (s *Repository) TenantBySlug(ctx context.Context, slug string) (*tenancydomain.TenantRef, error) {
	row, ok, err := tenant.New().Where(tenant.Slug.Eq(slug)).One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound.With("tenant", slug)
	}
	return tenantRef(row), nil
}

func (s *Repository) TenantByID(ctx context.Context, id string) (*tenancydomain.TenantRef, error) {
	tid, err := database.ParseUUID(id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	row, ok, err := tenant.New().Where(tenant.ID.Eq(tid)).One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound.With("tenant", id)
	}
	return tenantRef(row), nil
}

func (s *Repository) ListTenants(ctx context.Context) ([]tenancydomain.TenantRef, error) {
	rows, err := tenant.New().Order(tenant.Slug.Asc()).All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.TenantRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, *tenantRef(r))
	}
	return out, nil
}

func (s *Repository) CreateTenant(ctx context.Context, slug, name, parentID string) (*tenancydomain.TenantRef, error) {
	n := tenant.Create()
	n.SetSlug(slug)
	n.SetName(name)
	if parentID != "" {
		pid, err := database.ParseUUID(parentID)
		if err != nil {
			return nil, database.MapErr(err)
		}
		n.SetParentID(pid)
	}
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		// tenants_tree_guard's refusals name only companies in the caller's
		// own group, so its sentence is safe to pass on.
		return nil, database.MapErrSaying(err)
	}
	return tenantRef(row), nil
}

func tenantRef(r tenant.Row) *tenancydomain.TenantRef {
	ref := &tenancydomain.TenantRef{
		ID: database.UUIDStr(r.ID), Slug: r.Slug, Name: r.Name,
		Status: r.Status, CreatedAt: r.CreatedAt,
	}
	if r.ParentID.Valid {
		ref.ParentID = database.UUIDStr(r.ParentID.V)
	}
	return ref
}

// MoveTenant puts a company under another, or at the top with "". The tree
// guard refuses a loop or a group deeper than six levels, in its own words.
func (s *Repository) MoveTenant(ctx context.Context, id, parentID string) error {
	n, err := tenancyrquery.MoveTenant.Exec(ctx, s.ex(ctx), id, parentID)
	if err != nil {
		return database.MapErrSaying(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("tenant", id)
	}
	return nil
}

// CatalogVersion is the counter a gate snapshot is built from.
func (s *Repository) CatalogVersion(ctx context.Context, tenantID string) (int64, time.Time, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return 0, time.Time{}, database.MapErr(err)
	}
	row, ok, err := catalogversion.New().
		Where(catalogversion.TenantID.Eq(tid)).
		One(ctx, s.ex(ctx))
	if err != nil {
		return 0, time.Time{}, database.MapErr(err)
	}
	if !ok {
		return 0, time.Time{}, apperr.ErrNotFound.With("catalog_version", tenantID)
	}
	return row.Version, row.ChangedAt, nil
}

// BumpCatalogVersion invalidates every gate snapshot for a tenant.
func (s *Repository) BumpCatalogVersion(ctx context.Context, tenantID string) error {
	_, _, err := tenancyrquery.BumpCatalogVersion.One(ctx, s.ex(ctx), tenantID)
	return database.MapErr(err)
}

// UpdateTenant renames a tenant. The slug is deliberately not editable: it
// appears in URLs, tokens and every hosted page path, and changing it would
// break links that already exist in the world.
func (s *Repository) UpdateTenant(ctx context.Context, id, name string) error {
	n, err := tenancyrquery.UpdateTenant.Exec(ctx, s.ex(ctx), id, name)
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
	n, err := tenancyrquery.SetTenantStatus.Exec(ctx, s.ex(ctx), id, status)
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
	row, _, err := tenancyrquery.CountTenantIdentities.One(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(row.N), nil
}

// TenantStats counts what a tenant holds.
func (s *Repository) TenantStats(ctx context.Context, tenantID string) (*tenancydomain.TenantStats, error) {
	row, _, err := tenancyrquery.GetTenantStats.One(ctx, s.ex(ctx), tenantID)
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
