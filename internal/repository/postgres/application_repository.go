package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ApplicationBySlug(ctx context.Context, tenantID, slug string) (*repository.ApplicationRecord, error) {
	r, err := s.q(ctx).GetApplicationBySlug(ctx, gen.GetApplicationBySlugParams{
		TenantID: tenantID, Slug: slug,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.ApplicationRecord{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
		RedirectURIs: r.RedirectUris, BackchannelLogoutURI: deref(r.BackchannelLogoutUri),
		TokenFormat: r.TokenFormat, ClientSecretHash: deref(r.ClientSecretHash),
		AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
		AccessTokenTTLSecs: r.AccessTokenTtlSecs, RefreshTokenTTLSecs: r.RefreshTokenTtlSecs,
		ManifestVersion: int(r.ManifestVersion),
	}, nil
}

func (s *Store) ApplicationByID(ctx context.Context, tenantID, id string) (*repository.ApplicationRecord, error) {
	r, err := s.q(ctx).GetApplication(ctx, gen.GetApplicationParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.ApplicationRecord{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
		RedirectURIs: r.RedirectUris, BackchannelLogoutURI: deref(r.BackchannelLogoutUri),
		TokenFormat: r.TokenFormat, ClientSecretHash: deref(r.ClientSecretHash),
		AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
		ManifestVersion: int(r.ManifestVersion),
	}, nil
}

func (s *Store) ListApplications(ctx context.Context, tenantID string) ([]repository.ApplicationRecord, error) {
	rows, err := s.q(ctx).ListApplications(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.ApplicationRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.ApplicationRecord{
			ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
			RedirectURIs: r.RedirectUris, BackchannelLogoutURI: deref(r.BackchannelLogoutUri),
			TokenFormat: r.TokenFormat,
			AccessTokenTTL: r.AccessTokenTtl, RefreshTokenTTL: r.RefreshTokenTtl,
			ManifestVersion: int(r.ManifestVersion),
		})
	}
	return out, nil
}

func (s *Store) CreateApplication(ctx context.Context, tenantID string, a repository.ApplicationRecord) (string, error) {
	row, err := s.q(ctx).CreateApplication(ctx, gen.CreateApplicationParams{
		TenantID: tenantID, Slug: a.Slug, Name: a.Name, Kind: a.Kind,
		RedirectUris: emptyIfNil(a.RedirectURIs), BackchannelLogoutUri: a.BackchannelLogoutURI,
		TokenFormat:      orDefaultStr(a.TokenFormat, "v4.public"),
		ClientSecretHash: a.ClientSecretHash,
		AccessTokenTtl:   orDefaultStr(a.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl:  orDefaultStr(a.RefreshTokenTTL, "30 days"),
	})
	if err != nil {
		return "", mapErr(err)
	}
	return row.ID, nil
}

func (s *Store) UpdateApplication(ctx context.Context, tenantID string, a repository.ApplicationRecord) error {
	_, err := s.q(ctx).UpdateApplication(ctx, gen.UpdateApplicationParams{
		ID: a.ID, TenantID: tenantID, Name: a.Name, Status: orDefaultStr(a.Status, "active"),
		RedirectUris: emptyIfNil(a.RedirectURIs), BackchannelLogoutUri: a.BackchannelLogoutURI,
		TokenFormat:     orDefaultStr(a.TokenFormat, "v4.public"),
		AccessTokenTtl:  orDefaultStr(a.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl: orDefaultStr(a.RefreshTokenTTL, "30 days"),
	})
	return mapErr(err)
}

func (s *Store) SetClientSecretHash(ctx context.Context, tenantID, id, hash string) error {
	_, err := s.q(ctx).SetClientSecretHash(ctx, gen.SetClientSecretHashParams{
		ID: id, TenantID: tenantID, ClientSecretHash: optStr(hash),
	})
	return mapErr(err)
}

func (s *Store) BumpManifestVersion(ctx context.Context, applicationID string) (int, error) {
	v, err := s.q(ctx).BumpManifestVersion(ctx, applicationID)
	return int(v), mapErr(err)
}

func (s *Store) BackchannelApps(ctx context.Context, tenantID string) ([]string, []string, error) {
	rows, err := s.q(ctx).ListBackchannelApps(ctx, tenantID)
	if err != nil {
		return nil, nil, mapErr(err)
	}
	slugs := make([]string, 0, len(rows))
	uris := make([]string, 0, len(rows))
	for _, r := range rows {
		slugs = append(slugs, r.Slug)
		uris = append(uris, deref(r.BackchannelLogoutUri))
	}
	return slugs, uris, nil
}
