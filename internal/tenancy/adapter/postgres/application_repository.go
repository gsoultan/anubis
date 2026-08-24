package tenancypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/gen"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

func (s *Repository) ApplicationBySlug(ctx context.Context, tenantID, slug string) (*tenancydomain.ApplicationRecord, error) {
	r, err := s.q(ctx).GetApplicationBySlug(ctx, gen.GetApplicationBySlugParams{
		TenantID: tenantID, Slug: slug,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.ApplicationRecord{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
		RedirectURIs: r.RedirectUris, PostLogoutRedirectURIs: r.PostLogoutRedirectUris, BackchannelLogoutURI: database.Deref(r.BackchannelLogoutUri),
		TokenFormat: r.TokenFormat, ClientSecretHash: database.Deref(r.ClientSecretHash),
		AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
		AccessTokenTTLSecs: r.AccessTokenTtlSecs, RefreshTokenTTLSecs: r.RefreshTokenTtlSecs,
		ManifestVersion: int(r.ManifestVersion),
	}, nil
}

func (s *Repository) ApplicationByID(ctx context.Context, tenantID, id string) (*tenancydomain.ApplicationRecord, error) {
	r, err := s.q(ctx).GetApplication(ctx, gen.GetApplicationParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &tenancydomain.ApplicationRecord{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
		RedirectURIs: r.RedirectUris, PostLogoutRedirectURIs: r.PostLogoutRedirectUris, BackchannelLogoutURI: database.Deref(r.BackchannelLogoutUri),
		TokenFormat: r.TokenFormat, ClientSecretHash: database.Deref(r.ClientSecretHash),
		AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
		ManifestVersion: int(r.ManifestVersion),
	}, nil
}

// ListApplications is one keyset page of the TENANT's relying parties.
// Anubis's own two are excluded by the query — see the SQL for why.
func (s *Repository) ListApplications(ctx context.Context, tenantID, query, after string, pageSize int32) ([]tenancydomain.ApplicationRecord, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	rows, err := s.q(ctx).ListApplications(ctx, gen.ListApplicationsParams{
		TenantID: tenantID, Query: query, After: after, PageSize: pageSize,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.ApplicationRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.ApplicationRecord{
			ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
			RedirectURIs: r.RedirectUris, PostLogoutRedirectURIs: r.PostLogoutRedirectUris, BackchannelLogoutURI: database.Deref(r.BackchannelLogoutUri),
			TokenFormat:    r.TokenFormat,
			AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
			ManifestVersion: int(r.ManifestVersion),
		})
	}
	return out, nil
}

func (s *Repository) AllApplications(ctx context.Context, tenantID string) ([]tenancydomain.ApplicationRecord, error) {
	rows, err := s.q(ctx).AllApplications(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.ApplicationRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.ApplicationRecord{
			ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
			RedirectURIs: r.RedirectUris, PostLogoutRedirectURIs: r.PostLogoutRedirectUris, BackchannelLogoutURI: database.Deref(r.BackchannelLogoutUri),
			TokenFormat:    r.TokenFormat,
			AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
			ManifestVersion: int(r.ManifestVersion),
		})
	}
	return out, nil
}

func (s *Repository) CreateApplication(ctx context.Context, tenantID string, a tenancydomain.ApplicationRecord) (string, error) {
	row, err := s.q(ctx).CreateApplication(ctx, gen.CreateApplicationParams{
		TenantID: tenantID, Slug: a.Slug, Name: a.Name, Kind: a.Kind,
		RedirectUris:           database.EmptyIfNil(a.RedirectURIs),
		PostLogoutRedirectUris: database.EmptyIfNil(a.PostLogoutRedirectURIs), BackchannelLogoutUri: a.BackchannelLogoutURI,
		TokenFormat:      database.OrDefaultStr(a.TokenFormat, "v4.public"),
		ClientSecretHash: a.ClientSecretHash,
		AccessTokenTtl:   database.OrDefaultStr(a.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl:  database.OrDefaultStr(a.RefreshTokenTTL, "30 days"),
	})
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) UpdateApplication(ctx context.Context, tenantID string, a tenancydomain.ApplicationRecord) error {
	_, err := s.q(ctx).UpdateApplication(ctx, gen.UpdateApplicationParams{
		ID: a.ID, TenantID: tenantID, Name: a.Name, Status: database.OrDefaultStr(a.Status, "active"),
		RedirectUris:           database.EmptyIfNil(a.RedirectURIs),
		PostLogoutRedirectUris: database.EmptyIfNil(a.PostLogoutRedirectURIs), BackchannelLogoutUri: a.BackchannelLogoutURI,
		TokenFormat:     database.OrDefaultStr(a.TokenFormat, "v4.public"),
		AccessTokenTtl:  database.OrDefaultStr(a.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl: database.OrDefaultStr(a.RefreshTokenTTL, "30 days"),
	})
	return database.MapErr(err)
}

func (s *Repository) SetClientSecretHash(ctx context.Context, tenantID, id, hash string) error {
	_, err := s.q(ctx).SetClientSecretHash(ctx, gen.SetClientSecretHashParams{
		ID: id, TenantID: tenantID, ClientSecretHash: database.OptStr(hash),
	})
	return database.MapErr(err)
}

func (s *Repository) BumpManifestVersion(ctx context.Context, applicationID string) (int, error) {
	v, err := s.q(ctx).BumpManifestVersion(ctx, applicationID)
	return int(v), database.MapErr(err)
}

func (s *Repository) BackchannelApps(ctx context.Context, tenantID string) ([]string, []string, error) {
	rows, err := s.q(ctx).ListBackchannelApps(ctx, tenantID)
	if err != nil {
		return nil, nil, database.MapErr(err)
	}
	slugs := make([]string, 0, len(rows))
	uris := make([]string, 0, len(rows))
	for _, r := range rows {
		slugs = append(slugs, r.Slug)
		uris = append(uris, database.Deref(r.BackchannelLogoutUri))
	}
	return slugs, uris, nil
}

// CountApplications is the tenant's whole population, so a page can say
// "20 of 138" rather than implying it is everything there is.
func (s *Repository) CountApplications(ctx context.Context, tenantID string) (int, error) {
	n, err := s.q(ctx).CountApplications(ctx, tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(n), nil
}
