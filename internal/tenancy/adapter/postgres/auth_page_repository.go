package tenancypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/gen"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

func (s *Repository) ListAuthPages(ctx context.Context, tenantID, kind string) ([]tenancydomain.AuthPage, error) {
	rows, err := s.q(ctx).ListAuthPages(ctx, gen.ListAuthPagesParams{
		TenantID: tenantID, Kind: database.OptStr(kind),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.AuthPage, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.AuthPage{
			ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug,
			Name: r.Name, Status: r.Status, IsDefault: r.IsDefault,
			ApplicationID:   database.Deref(r.ApplicationID),
			ApplicationSlug: database.Deref(r.ApplicationSlug),
			Config:          r.Config, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Repository) AuthPage(ctx context.Context, tenantID, id string) (*tenancydomain.AuthPage, error) {
	r, err := s.q(ctx).GetAuthPage(ctx, gen.GetAuthPageParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID:   database.Deref(r.ApplicationID),
		ApplicationSlug: database.Deref(r.ApplicationSlug),
		Config:          r.Config, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

func (s *Repository) AuthPageBySlug(ctx context.Context, tenantID, kind, slug string) (*tenancydomain.AuthPage, error) {
	r, err := s.q(ctx).GetAuthPageBySlug(ctx, gen.GetAuthPageBySlugParams{
		TenantID: tenantID, Kind: kind, Slug: slug,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID:   database.Deref(r.ApplicationID),
		ApplicationSlug: database.Deref(r.ApplicationSlug),
		RealmID:         database.Deref(r.RealmID),
		RealmCode:       database.Deref(r.RealmCode), Config: r.Config,
	}, nil
}

func (s *Repository) DefaultAuthPage(ctx context.Context, tenantID, kind string) (*tenancydomain.AuthPage, error) {
	r, err := s.q(ctx).GetDefaultAuthPage(ctx, gen.GetDefaultAuthPageParams{
		TenantID: tenantID, Kind: kind,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID:   database.Deref(r.ApplicationID),
		ApplicationSlug: database.Deref(r.ApplicationSlug),
		RealmID:         database.Deref(r.RealmID),
		RealmCode:       database.Deref(r.RealmCode), Config: r.Config,
	}, nil
}

func (s *Repository) AuthPageForApplication(ctx context.Context, tenantID, kind, applicationID string) (*tenancydomain.AuthPage, error) {
	r, err := s.q(ctx).GetAuthPageForApplication(ctx, gen.GetAuthPageForApplicationParams{
		TenantID: tenantID, Kind: kind, ApplicationID: database.OptStr(applicationID),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID: database.Deref(r.ApplicationID),
		RealmID:       database.Deref(r.RealmID), Config: r.Config,
	}, nil
}

// AuthPageForRealm is the population's own door. resolvePage tries it after
// the application binding and before the tenant default, so an application
// that configured its own page keeps it.
func (s *Repository) AuthPageForRealm(ctx context.Context, tenantID, kind, realmID string) (*tenancydomain.AuthPage, error) {
	r, err := s.q(ctx).GetAuthPageForRealm(ctx, gen.GetAuthPageForRealmParams{
		TenantID: tenantID, Kind: kind, RealmID: database.OptStr(realmID),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID: database.Deref(r.ApplicationID),
		RealmID:       database.Deref(r.RealmID), Config: r.Config,
	}, nil
}

func (s *Repository) CreateAuthPage(ctx context.Context, tenantID string, in tenancydomain.AuthPageInput) (string, error) {
	id, err := s.q(ctx).CreateAuthPage(ctx, gen.CreateAuthPageParams{
		TenantID: tenantID, Kind: in.Kind, Slug: in.Slug, Name: in.Name,
		Status:        database.OrDefaultStr(in.Status, "active"),
		ApplicationID: database.OptStr(in.ApplicationID),
		RealmID:       database.OptStr(in.RealmID),
		Config:        database.OrEmptyJSON(in.Config),
	})
	return id, database.MapErr(err)
}

func (s *Repository) UpdateAuthPage(ctx context.Context, tenantID string, in tenancydomain.AuthPageInput) error {
	n, err := s.q(ctx).UpdateAuthPage(ctx, gen.UpdateAuthPageParams{
		ID: in.ID, TenantID: tenantID, Name: in.Name,
		Status:        database.OrDefaultStr(in.Status, "active"),
		ApplicationID: database.OptStr(in.ApplicationID),
		RealmID:       database.OptStr(in.RealmID),
		Config:        database.OrEmptyJSON(in.Config),
	})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

func (s *Repository) DeleteAuthPage(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).DeleteAuthPage(ctx, gen.DeleteAuthPageParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		// Either it does not exist or it is the default, which the query
		// refuses to delete: /v1/authorize must always have a page to render.
		return database.NotFound()
	}
	return nil
}

func (s *Repository) SetDefaultAuthPage(ctx context.Context, tenantID, kind, id string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).ClearDefaultAuthPage(ctx, gen.ClearDefaultAuthPageParams{
			TenantID: tenantID, Kind: kind,
		}); err != nil {
			return database.MapErr(err)
		}
		n, err := s.q(ctx).SetDefaultAuthPage(ctx, gen.SetDefaultAuthPageParams{
			ID: id, TenantID: tenantID,
		})
		if err != nil {
			return database.MapErr(err)
		}
		if n == 0 {
			return database.NotFound()
		}
		return nil
	})
}
