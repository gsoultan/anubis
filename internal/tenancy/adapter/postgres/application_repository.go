package tenancypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rgen/application"
	tenancyrquery "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rquery"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

func (s *Repository) ApplicationBySlug(ctx context.Context, tenantID, slug string) (*tenancydomain.ApplicationRecord, error) {
	r, _, err := tenancyrquery.GetApplicationBySlug.One(ctx, s.ex(ctx), tenantID, slug)
	if err != nil {
		return nil, database.MapErr(err)
	}
	rec := appRecord(tenancyrquery.ApplicationRow{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
		RedirectUris: r.RedirectUris, PostLogoutRedirectUris: r.PostLogoutRedirectUris,
		BackchannelLogoutURI: r.BackchannelLogoutURI, TokenFormat: r.TokenFormat,
		ClientSecretHash: r.ClientSecretHash, ManifestVersion: r.ManifestVersion,
		AccessTokenTtl: r.AccessTokenTtl, RefreshTokenTtl: r.RefreshTokenTtl,
	})
	// Only this lookup carries the seconds: it is the one on the token path.
	rec.AccessTokenTTLSecs = r.AccessTokenTtlSecs
	rec.RefreshTokenTTLSecs = r.RefreshTokenTtlSecs
	return &rec, nil
}

func (s *Repository) ApplicationByID(ctx context.Context, tenantID, id string) (*tenancydomain.ApplicationRecord, error) {
	r, _, err := tenancyrquery.GetApplication.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	rec := appRecord(r)
	return &rec, nil
}

// ListApplications is one keyset page of the TENANT's relying parties.
func (s *Repository) ListApplications(ctx context.Context, tenantID, query, after string, pageSize int32) ([]tenancydomain.ApplicationRecord, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	rows, err := tenancyrquery.ListApplications.Query(ctx, s.ex(ctx),
		tenantID, query, after, pageSize)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return appRecords(rows), nil
}

// AllApplications is the unpaged read for internal checks: validating a
// post-logout redirect walks every registered URI, and paging that would
// silently reject valid redirects past the first page.
func (s *Repository) AllApplications(ctx context.Context, tenantID string) ([]tenancydomain.ApplicationRecord, error) {
	rows, err := tenancyrquery.AllApplications.Query(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return appRecords(rows), nil
}

func appRecords(rows []tenancyrquery.ApplicationRow) []tenancydomain.ApplicationRecord {
	out := make([]tenancydomain.ApplicationRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, appRecord(r))
	}
	return out
}

func appRecord(r tenancyrquery.ApplicationRow) tenancydomain.ApplicationRecord {
	return tenancydomain.ApplicationRecord{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Kind: r.Kind, Status: r.Status,
		RedirectURIs:           r.RedirectUris,
		PostLogoutRedirectURIs: r.PostLogoutRedirectUris,
		BackchannelLogoutURI:   nstr(r.BackchannelLogoutURI),
		TokenFormat:            r.TokenFormat,
		ClientSecretHash:       nstr(r.ClientSecretHash),
		AccessTokenTTL:         r.AccessTokenTtl,
		RefreshTokenTTL:        r.RefreshTokenTtl,
		ManifestVersion:        int(r.ManifestVersion),
	}
}

func (s *Repository) CreateApplication(ctx context.Context, tenantID string, a tenancydomain.ApplicationRecord) (string, error) {
	row, _, err := tenancyrquery.CreateApplication.One(ctx, s.ex(ctx),
		tenantID, a.Slug, a.Name, a.Kind,
		database.EmptyIfNil(a.RedirectURIs),
		database.EmptyIfNil(a.PostLogoutRedirectURIs),
		a.BackchannelLogoutURI,
		database.OrDefaultStr(a.TokenFormat, "v4.public"),
		a.ClientSecretHash,
		database.OrDefaultStr(a.AccessTokenTTL, "10 minutes"),
		database.OrDefaultStr(a.RefreshTokenTTL, "30 days"))
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) UpdateApplication(ctx context.Context, tenantID string, a tenancydomain.ApplicationRecord) error {
	_, err := tenancyrquery.UpdateApplication.Exec(ctx, s.ex(ctx),
		a.ID, tenantID, a.Name, database.OrDefaultStr(a.Status, "active"),
		database.EmptyIfNil(a.RedirectURIs),
		database.EmptyIfNil(a.PostLogoutRedirectURIs),
		a.BackchannelLogoutURI,
		database.OrDefaultStr(a.TokenFormat, "v4.public"),
		database.OrDefaultStr(a.AccessTokenTTL, "10 minutes"),
		database.OrDefaultStr(a.RefreshTokenTTL, "30 days"))
	return database.MapErr(err)
}

func (s *Repository) SetClientSecretHash(ctx context.Context, tenantID, id, hash string) error {
	_, err := tenancyrquery.SetClientSecretHash.Exec(ctx, s.ex(ctx), id, tenantID, hash)
	return database.MapErr(err)
}

// BumpManifestVersion publishes a new configuration generation and returns it.
func (s *Repository) BumpManifestVersion(ctx context.Context, applicationID string) (int, error) {
	row, _, err := tenancyrquery.BumpManifestVersion.One(ctx, s.ex(ctx), applicationID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(row.ManifestVersion), nil
}

// BackchannelApps lists the applications that asked to be told about a logout.
//
// Active only, and only those with a URI: a disabled application must not keep
// receiving a tenant's sign-out notifications.
func (s *Repository) BackchannelApps(ctx context.Context, tenantID string) ([]string, []string, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, nil, database.MapErr(err)
	}
	rows, err := application.New().
		Where(application.TenantID.Eq(tid),
			application.BackchannelLogoutURI.IsNotNull(),
			application.Status.Eq("active")).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, nil, database.MapErr(err)
	}
	slugs := make([]string, 0, len(rows))
	uris := make([]string, 0, len(rows))
	for _, r := range rows {
		uri, _ := r.BackchannelLogoutURI.Get()
		slugs = append(slugs, r.Slug)
		uris = append(uris, uri)
	}
	return slugs, uris, nil
}

func (s *Repository) CountApplications(ctx context.Context, tenantID string) (int, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	n, err := application.New().Where(application.TenantID.Eq(tid)).Count(ctx, s.ex(ctx))
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(n), nil
}
