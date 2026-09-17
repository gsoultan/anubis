package tenancypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rgen/authpage"
	tenancyrquery "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rquery"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	"github.com/gsoultan/storm/runtime"
)

func (s *Repository) ListAuthPages(ctx context.Context, tenantID, kind string) ([]tenancydomain.AuthPage, error) {
	// An empty kind means "every kind". The query tests $2 IS NULL, so the
	// absence has to arrive as NULL rather than as '' — which would match no
	// page at all and look like a tenant with nothing configured.
	var k any
	if kind != "" {
		k = kind
	}
	rows, err := tenancyrquery.ListAuthPages.Query(ctx, s.ex(ctx), tenantID, k)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.AuthPage, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.AuthPage{
			ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug,
			Name: r.Name, Status: r.Status, IsDefault: r.IsDefault,
			ApplicationID:   nstr(r.ApplicationID),
			ApplicationSlug: nstr(r.ApplicationSlug),
			Config:          []byte(r.Config),
			CreatedAt:       r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Repository) AuthPage(ctx context.Context, tenantID, id string) (*tenancydomain.AuthPage, error) {
	r, ok, err := tenancyrquery.GetAuthPage.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID:   nstr(r.ApplicationID),
		ApplicationSlug: nstr(r.ApplicationSlug),
		Config:          []byte(r.Config),
		CreatedAt:       r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

// AuthPageBySlug is the public render path. Active only: a disabled page must
// 404 rather than render, so a retired design cannot be resurrected by anyone
// who kept the link.
func (s *Repository) AuthPageBySlug(ctx context.Context, tenantID, kind, slug string) (*tenancydomain.AuthPage, error) {
	r, ok, err := tenancyrquery.GetAuthPageBySlug.One(ctx, s.ex(ctx), tenantID, kind, slug)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return resolvedPage(r), nil
}

func (s *Repository) DefaultAuthPage(ctx context.Context, tenantID, kind string) (*tenancydomain.AuthPage, error) {
	r, ok, err := tenancyrquery.GetDefaultAuthPage.One(ctx, s.ex(ctx), tenantID, kind)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return resolvedPage(r), nil
}

func resolvedPage(r tenancyrquery.ResolvedPageRow) *tenancydomain.AuthPage {
	return &tenancydomain.AuthPage{
		ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Slug: r.Slug, Name: r.Name,
		Status: r.Status, IsDefault: r.IsDefault,
		ApplicationID:   nstr(r.ApplicationID),
		ApplicationSlug: nstr(r.ApplicationSlug),
		Config:          []byte(r.Config),
	}
}

// AuthPageForApplication is an application-initiated flow's own page.
//
// No join: the resolver only needs the page, and the caller already knows
// which application it asked about.
func (s *Repository) AuthPageForApplication(ctx context.Context, tenantID, kind, applicationID string) (*tenancydomain.AuthPage, error) {
	tid, appID, err := twoUUIDs(tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	return s.onePage(ctx, authpage.New().Where(
		authpage.TenantID.Eq(tid),
		authpage.Kind.Eq(kind),
		authpage.ApplicationID.Eq(appID),
		authpage.Status.Eq("active")))
}

// AuthPageForRealm is the population's own door. The resolver tries it AFTER
// the application binding and BEFORE the tenant default, so an application
// that configured its own page keeps it; this fills the gap that used to fall
// straight through to the default.
func (s *Repository) AuthPageForRealm(ctx context.Context, tenantID, kind, realmID string) (*tenancydomain.AuthPage, error) {
	tid, rid, err := twoUUIDs(tenantID, realmID)
	if err != nil {
		return nil, err
	}
	return s.onePage(ctx, authpage.New().Where(
		authpage.TenantID.Eq(tid),
		authpage.Kind.Eq(kind),
		authpage.RealmID.Eq(rid),
		authpage.Status.Eq("active")))
}

func twoUUIDs(a, b string) ([16]byte, [16]byte, error) {
	x, err := database.ParseUUID(a)
	if err != nil {
		return x, x, database.MapErr(err)
	}
	y, err := database.ParseUUID(b)
	if err != nil {
		return x, y, database.MapErr(err)
	}
	return x, y, nil
}

func (s *Repository) onePage(ctx context.Context, q authpage.Query) (*tenancydomain.AuthPage, error) {
	r, ok, err := q.One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	appID, _ := r.ApplicationID.Get()
	page := &tenancydomain.AuthPage{
		ID: database.UUIDStr(r.ID), TenantID: database.UUIDStr(r.TenantID),
		Kind: r.Kind, Slug: r.Slug, Name: r.Name, Status: r.Status,
		IsDefault: r.IsDefault, Config: []byte(r.Config),
	}
	if _, present := r.ApplicationID.Get(); present {
		page.ApplicationID = database.UUIDStr(appID)
	}
	return page, nil
}

func (s *Repository) CreateAuthPage(ctx context.Context, tenantID string, in tenancydomain.AuthPageInput) (string, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return "", database.MapErr(err)
	}
	n := authpage.Create()
	n.SetTenantID(tid)
	n.SetKind(in.Kind)
	n.SetSlug(in.Slug)
	n.SetName(in.Name)
	n.SetStatus(database.OrDefaultStr(in.Status, "active"))
	n.SetConfig(runtime.JSON(database.OrEmptyJSON(in.Config)))
	// A page binds to an application OR a realm OR neither; the check
	// constraint forbids both, so each absence is NULL rather than a zero id.
	if err := setOptID(n.SetApplicationID, n.SetApplicationIDNull, in.ApplicationID); err != nil {
		return "", database.MapErr(err)
	}
	if err := setOptID(n.SetRealmID, n.SetRealmIDNull, in.RealmID); err != nil {
		return "", database.MapErr(err)
	}
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

func setOptID(set func([16]byte), setNull func(), v string) error {
	if v == "" {
		setNull()
		return nil
	}
	u, err := database.ParseUUID(v)
	if err != nil {
		return err
	}
	set(u)
	return nil
}

func (s *Repository) UpdateAuthPage(ctx context.Context, tenantID string, in tenancydomain.AuthPageInput) error {
	var appID, realmID any
	if in.ApplicationID != "" {
		appID = in.ApplicationID
	}
	if in.RealmID != "" {
		realmID = in.RealmID
	}
	n, err := tenancyrquery.UpdateAuthPage.Exec(ctx, s.ex(ctx),
		in.ID, tenantID, in.Name, database.OrDefaultStr(in.Status, "active"),
		appID, realmID, database.OrEmptyJSON(in.Config))
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

// DeleteAuthPage refuses the default: deleting it would leave /v1/authorize
// with no page to render. Promote another page first.
func (s *Repository) DeleteAuthPage(ctx context.Context, tenantID, id string) error {
	n, err := tenancyrquery.DeleteAuthPage.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		// Either it does not exist or it is the default, which this refuses
		// to delete.
		return database.NotFound()
	}
	return nil
}

// SetDefaultAuthPage promotes one page and demotes whichever held the slot.
//
// Both inside ONE transaction, because the partial unique index allows exactly
// one default per kind: promoting without demoting first is a constraint
// violation rather than a swap.
func (s *Repository) SetDefaultAuthPage(ctx context.Context, tenantID, kind, id string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := tenancyrquery.ClearDefaultAuthPage.Exec(ctx, s.ex(ctx), tenantID, kind); err != nil {
			return database.MapErr(err)
		}
		n, err := tenancyrquery.SetDefaultAuthPage.Exec(ctx, s.ex(ctx), id, tenantID)
		if err != nil {
			return database.MapErr(err)
		}
		if n == 0 {
			return database.NotFound()
		}
		return nil
	})
}
