package tenancyrpc

import (
	"context"
	"encoding/hex"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/mw"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancyapp "github.com/gsoultan/anubis/internal/tenancy/app"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancysvc "github.com/gsoultan/anubis/internal/tenancy/service"
)

// TenantAdminHandler implements anubisv1connect.TenantAdminServiceHandler.
type TenantAdminHandler struct {
	svc tenancysvc.TenantAdminService
	f   mw.Factory
	// issuer and tenantSlug build the public URL a page is served at, so the
	// console can show and copy it rather than reconstruct it.
	issuer     string
	tenantSlug string
}

func NewTenantAdminHandler(svc tenancysvc.TenantAdminService, f mw.Factory,
	issuer, tenantSlug string) *TenantAdminHandler {
	return &TenantAdminHandler{svc: svc, f: f, issuer: issuer, tenantSlug: tenantSlug}
}

var _ anubisv1connect.TenantAdminServiceHandler = (*TenantAdminHandler)(nil)

func realmProto(r identitydomain.RealmRecord) *anubisv1.Realm {
	return &anubisv1.Realm{
		Id: r.ID, Code: r.Code, Kind: r.Kind, DisplayName: r.DisplayName,
		MinAssurance: int32(r.MinAssurance), SelfRegistration: r.SelfRegistration,
		EmailVerificationRequired: r.EmailVerification, PiiEncryption: r.PIIEncryption,
		AllowedFactors: r.AllowedFactors, RequiredFactors: r.RequiredFactors,
		SessionTtl: r.SessionTTL, AccessTokenTtl: r.AccessTokenTTL,
		RefreshTokenTtl: r.RefreshTokenTTL, DefaultRetention: r.DefaultRetention,
		PasswordPolicyJson:      string(r.PasswordPolicy),
		FactorEnrolmentDeadline: unixOrZero(r.FactorEnrolmentDeadline),
	}
}

func realmRecord(r *anubisv1.Realm) identitydomain.RealmRecord {
	return identitydomain.RealmRecord{
		ID: r.Id, Code: r.Code, Kind: r.Kind, DisplayName: r.DisplayName,
		MinAssurance: int(r.MinAssurance), SelfRegistration: r.SelfRegistration,
		EmailVerification: r.EmailVerificationRequired, PIIEncryption: r.PiiEncryption,
		AllowedFactors: r.AllowedFactors, RequiredFactors: r.RequiredFactors,
		SessionTTL: r.SessionTtl, AccessTokenTTL: r.AccessTokenTtl,
		RefreshTokenTTL: r.RefreshTokenTtl, DefaultRetention: r.DefaultRetention,
		PasswordPolicy:          []byte(r.PasswordPolicyJson),
		FactorEnrolmentDeadline: timeOrNil(r.FactorEnrolmentDeadline),
	}
}

// 0 is "not in force" on the wire and nil in the record: a realm that has
// never started enforcing enrolment and one that stopped are the same state,
// and both must round-trip through an ordinary UpdateRealm without a console
// accidentally setting a deadline of 1970.
func unixOrZero(t *time.Time) int64 {
	if t == nil || t.IsZero() {
		return 0
	}
	return t.Unix()
}

func timeOrNil(sec int64) *time.Time {
	if sec == 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func appProto(a *tenancydomain.ApplicationRecord) *anubisv1.Application {
	return &anubisv1.Application{
		Id: a.ID, Slug: a.Slug, Name: a.Name, Kind: a.Kind, Status: a.Status,
		RedirectUris: a.RedirectURIs, PostLogoutRedirectUris: a.PostLogoutRedirectURIs,
		BackchannelLogoutUri: a.BackchannelLogoutURI,
		TokenFormat:          a.TokenFormat, AccessTokenTtl: a.AccessTokenTTL,
		RefreshTokenTtl: a.RefreshTokenTTL, ManifestVersion: int32(a.ManifestVersion),
	}
}

func (h *TenantAdminHandler) ListTenants(ctx context.Context, _ *connect.Request[anubisv1.ListTenantsRequest]) (*connect.Response[anubisv1.ListTenantsResponse], error) {
	out, err := h.f.Do(ctx, "admin.tenant.list", func(ctx context.Context) (any, error) {
		return h.svc.ListTenants(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListTenantsResponse{}
	for _, t := range out.([]tenancydomain.TenantRef) {
		resp.Tenants = append(resp.Tenants, &anubisv1.Tenant{
			Id: t.ID, Slug: t.Slug, Name: t.Name, Status: t.Status, CreatedAt: t.CreatedAt.Unix(),
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) CreateTenant(ctx context.Context, req *connect.Request[anubisv1.CreateTenantRequest]) (*connect.Response[anubisv1.CreateTenantResponse], error) {
	out, err := h.f.Do(ctx, "admin.tenant.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateTenant(ctx, req.Msg.Slug, req.Msg.Name)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	t := out.(*tenancydomain.TenantRef)
	return connect.NewResponse(&anubisv1.CreateTenantResponse{
		Tenant: &anubisv1.Tenant{Id: t.ID, Slug: t.Slug, Name: t.Name, Status: t.Status},
	}), nil
}

func (h *TenantAdminHandler) ListRealms(ctx context.Context, _ *connect.Request[anubisv1.ListRealmsRequest]) (*connect.Response[anubisv1.ListRealmsResponse], error) {
	out, err := h.f.Do(ctx, "admin.realm.list", func(ctx context.Context) (any, error) {
		return h.svc.ListRealms(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListRealmsResponse{}
	for _, r := range out.([]identitydomain.RealmRecord) {
		resp.Realms = append(resp.Realms, realmProto(r))
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) CreateRealm(ctx context.Context, req *connect.Request[anubisv1.CreateRealmRequest]) (*connect.Response[anubisv1.CreateRealmResponse], error) {
	out, err := h.f.Do(ctx, "admin.realm.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateRealm(ctx, realmRecord(req.Msg.Realm))
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateRealmResponse{
		Realm: realmProto(*out.(*identitydomain.RealmRecord)),
	}), nil
}

func (h *TenantAdminHandler) UpdateRealm(ctx context.Context, req *connect.Request[anubisv1.UpdateRealmRequest]) (*connect.Response[anubisv1.UpdateRealmResponse], error) {
	out, err := h.f.Do(ctx, "admin.realm.update", func(ctx context.Context) (any, error) {
		return h.svc.UpdateRealm(ctx, realmRecord(req.Msg.Realm))
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateRealmResponse{
		Realm: realmProto(*out.(*identitydomain.RealmRecord)),
	}), nil
}

func (h *TenantAdminHandler) ListRealmCategories(ctx context.Context, req *connect.Request[anubisv1.ListRealmCategoriesRequest]) (*connect.Response[anubisv1.ListRealmCategoriesResponse], error) {
	out, err := h.f.Do(ctx, "admin.realm.categories", func(ctx context.Context) (any, error) {
		return h.svc.ListRealmCategories(ctx, req.Msg.RealmId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListRealmCategoriesResponse{}
	for _, c := range out.([]identitydomain.RealmCategoryRecord) {
		resp.Categories = append(resp.Categories, &anubisv1.RealmCategory{
			Id: c.ID, RealmId: c.RealmID, Code: c.Code,
			DisplayName: c.DisplayName, SortOrder: int32(c.SortOrder),
			IdentityCount: c.IdentityCount,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) CreateRealmCategory(ctx context.Context, req *connect.Request[anubisv1.CreateRealmCategoryRequest]) (*connect.Response[anubisv1.CreateRealmCategoryResponse], error) {
	out, err := h.f.Do(ctx, "admin.realm.category_create", func(ctx context.Context) (any, error) {
		c := req.Msg.Category
		return h.svc.CreateRealmCategory(ctx, identitydomain.RealmCategoryRecord{
			RealmID: c.RealmId, Code: c.Code, DisplayName: c.DisplayName,
			SortOrder: int(c.SortOrder),
		})
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	c := out.(*identitydomain.RealmCategoryRecord)
	return connect.NewResponse(&anubisv1.CreateRealmCategoryResponse{
		Category: &anubisv1.RealmCategory{
			Id: c.ID, RealmId: c.RealmID, Code: c.Code,
			DisplayName: c.DisplayName, SortOrder: int32(c.SortOrder),
		},
	}), nil
}

func (h *TenantAdminHandler) ListApplications(ctx context.Context, req *connect.Request[anubisv1.ListApplicationsRequest]) (*connect.Response[anubisv1.ListApplicationsResponse], error) {
	type page struct {
		apps  []tenancydomain.ApplicationRecord
		total int
	}
	size := int(req.Msg.PageSize)
	out, err := h.f.Do(ctx, "admin.application.list", func(ctx context.Context) (any, error) {
		apps, total, lerr := h.svc.ListApplications(ctx, req.Msg.Query, req.Msg.PageToken, size)
		return page{apps: apps, total: total}, lerr
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	pg := out.(page)
	resp := &anubisv1.ListApplicationsResponse{Total: int32(pg.total)}
	for i := range pg.apps {
		a := pg.apps[i]
		resp.Applications = append(resp.Applications, appProto(&a))
	}
	// A full page means there is probably another; the cursor is the last
	// slug, which is what the query orders by.
	if size > 0 && len(pg.apps) == size {
		resp.NextPageToken = pg.apps[len(pg.apps)-1].Slug
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) CreateApplication(ctx context.Context, req *connect.Request[anubisv1.CreateApplicationRequest]) (*connect.Response[anubisv1.CreateApplicationResponse], error) {
	out, err := h.f.Do(ctx, "admin.application.create", func(ctx context.Context) (any, error) {
		a := req.Msg.Application
		rec, secret, err := h.svc.CreateApplication(ctx, tenancydomain.ApplicationRecord{
			Slug: a.Slug, Name: a.Name, Kind: a.Kind,
			RedirectURIs: a.RedirectUris, PostLogoutRedirectURIs: a.PostLogoutRedirectUris,
			BackchannelLogoutURI: a.BackchannelLogoutUri,
			TokenFormat:          a.TokenFormat, AccessTokenTTL: a.AccessTokenTtl,
			RefreshTokenTTL: a.RefreshTokenTtl,
		})
		if err != nil {
			return nil, err
		}
		return &anubisv1.CreateApplicationResponse{
			Application: appProto(rec), ClientSecret: secret,
		}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.CreateApplicationResponse)), nil
}

func (h *TenantAdminHandler) UpdateApplication(ctx context.Context, req *connect.Request[anubisv1.UpdateApplicationRequest]) (*connect.Response[anubisv1.UpdateApplicationResponse], error) {
	out, err := h.f.Do(ctx, "admin.application.update", func(ctx context.Context) (any, error) {
		a := req.Msg.Application
		return h.svc.UpdateApplication(ctx, tenancydomain.ApplicationRecord{
			ID: a.Id, Slug: a.Slug, Name: a.Name, Status: a.Status,
			RedirectURIs: a.RedirectUris, PostLogoutRedirectURIs: a.PostLogoutRedirectUris,
			BackchannelLogoutURI: a.BackchannelLogoutUri,
			TokenFormat:          a.TokenFormat, AccessTokenTTL: a.AccessTokenTtl,
			RefreshTokenTTL: a.RefreshTokenTtl,
		})
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateApplicationResponse{
		Application: appProto(out.(*tenancydomain.ApplicationRecord)),
	}), nil
}

func (h *TenantAdminHandler) RotateClientSecret(ctx context.Context, req *connect.Request[anubisv1.RotateClientSecretRequest]) (*connect.Response[anubisv1.RotateClientSecretResponse], error) {
	out, err := h.f.Do(ctx, "admin.application.rotate_secret", func(ctx context.Context) (any, error) {
		return h.svc.RotateClientSecret(ctx, req.Msg.ApplicationId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RotateClientSecretResponse{ClientSecret: out.(string)}), nil
}

func (h *TenantAdminHandler) ListRoutePolicies(ctx context.Context, req *connect.Request[anubisv1.ListRoutePoliciesRequest]) (*connect.Response[anubisv1.ListRoutePoliciesResponse], error) {
	out, err := h.f.Do(ctx, "admin.route.list", func(ctx context.Context) (any, error) {
		return h.svc.ListRoutePolicies(ctx, req.Msg.ApplicationSlug)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListRoutePoliciesResponse{}
	for _, rp := range out.([]tenancydomain.RoutePolicyRecord) {
		resp.Policies = append(resp.Policies, &anubisv1.RoutePolicy{
			Id: rp.ID, ApplicationSlug: rp.AppSlug, Priority: int32(rp.Priority),
			Effect: rp.Effect, PathPattern: rp.PathPattern, HostPattern: rp.HostPattern,
			Methods: rp.Methods, PermissionKey: rp.PermissionKey,
			ScopeBindingsJson: string(rp.ScopeBindings),
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) QueryAudit(ctx context.Context, req *connect.Request[anubisv1.QueryAuditRequest]) (*connect.Response[anubisv1.QueryAuditResponse], error) {
	out, err := h.f.Do(ctx, "admin.audit.query", func(ctx context.Context) (any, error) {
		q := auditdomain.AuditQuery{
			ActorID: req.Msg.ActorId, Action: req.Msg.Action,
			Limit: int(req.Msg.PageSize),
		}
		if req.Msg.From > 0 {
			t := time.Unix(req.Msg.From, 0)
			q.From = &t
		}
		if req.Msg.To > 0 {
			t := time.Unix(req.Msg.To, 0)
			q.To = &t
		}
		list, next, err := h.svc.QueryAudit(ctx, q)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.QueryAuditResponse{NextPageToken: next}
		for _, a := range list {
			resp.Entries = append(resp.Entries, &anubisv1.AuditEntry{
				OccurredAt: a.OccurredAt.Unix(), Id: a.ID, Seq: a.Seq,
				ActorId: a.ActorID, ActorKind: a.ActorKind, TargetId: a.TargetID,
				SessionId: a.SessionID, Action: a.Action, Result: a.Result,
				Ip: a.IP, DetailJson: string(a.Detail),
				EntryHash: hex.EncodeToString(a.EntryHash),
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.QueryAuditResponse)), nil
}

func (h *TenantAdminHandler) VerifyAuditChain(ctx context.Context, req *connect.Request[anubisv1.VerifyAuditChainRequest]) (*connect.Response[anubisv1.VerifyAuditChainResponse], error) {
	out, err := h.f.Do(ctx, "admin.audit.verify", func(ctx context.Context) (any, error) {
		var from, to *time.Time
		if req.Msg.From > 0 {
			t := time.Unix(req.Msg.From, 0)
			from = &t
		}
		if req.Msg.To > 0 {
			t := time.Unix(req.Msg.To, 0)
			to = &t
		}
		checked, brokenAt, err := h.svc.VerifyAuditChain(ctx, from, to)
		if err != nil {
			return nil, err
		}
		return &anubisv1.VerifyAuditChainResponse{
			Ok: brokenAt == 0, Checked: checked, BrokenAtSeq: brokenAt,
		}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.VerifyAuditChainResponse)), nil
}

func (h *TenantAdminHandler) ListSigningKeys(ctx context.Context, _ *connect.Request[anubisv1.ListSigningKeysRequest]) (*connect.Response[anubisv1.ListSigningKeysResponse], error) {
	out, err := h.f.Do(ctx, "admin.key.list", func(ctx context.Context) (any, error) {
		return h.svc.ListSigningKeys(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListSigningKeysResponse{}
	for _, k := range out.([]authdomain.KeyRecord) {
		resp.Keys = append(resp.Keys, &anubisv1.SigningKey{
			Kid: k.Kid, Alg: k.Alg, Status: k.Status, Purpose: k.Purpose,
			NotBefore: k.NotBefore.Unix(), NotAfter: k.NotAfter.Unix(),
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) RotateSigningKey(ctx context.Context, req *connect.Request[anubisv1.RotateSigningKeyRequest]) (*connect.Response[anubisv1.RotateSigningKeyResponse], error) {
	out, err := h.f.Do(ctx, "admin.key.rotate", func(ctx context.Context) (any, error) {
		return h.svc.RotateSigningKey(ctx, req.Msg.Purpose)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	k := out.(*authdomain.KeyRecord)
	return connect.NewResponse(&anubisv1.RotateSigningKeyResponse{
		NewKey: &anubisv1.SigningKey{
			Kid: k.Kid, Alg: k.Alg, Status: k.Status, Purpose: k.Purpose,
			NotBefore: k.NotBefore.Unix(), NotAfter: k.NotAfter.Unix(),
		},
	}), nil
}

func (h *TenantAdminHandler) GetCatalogVersion(ctx context.Context, _ *connect.Request[anubisv1.GetCatalogVersionRequest]) (*connect.Response[anubisv1.GetCatalogVersionResponse], error) {
	out, err := h.f.Do(ctx, "admin.catalog.version", func(ctx context.Context) (any, error) {
		v, at, err := h.svc.GetCatalogVersion(ctx)
		if err != nil {
			return nil, err
		}
		return &anubisv1.GetCatalogVersionResponse{Version: v, ChangedAt: at.Unix()}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.GetCatalogVersionResponse)), nil
}

func (h *TenantAdminHandler) GetSigninPage(ctx context.Context, _ *connect.Request[anubisv1.GetSigninPageRequest]) (*connect.Response[anubisv1.GetSigninPageResponse], error) {
	out, err := h.f.Do(ctx, "admin.signin.get", func(ctx context.Context) (any, error) {
		cfg, at, err := h.svc.GetSigninPage(ctx)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.GetSigninPageResponse{ConfigJson: string(cfg)}
		if !at.IsZero() {
			resp.UpdatedAt = at.Unix()
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.GetSigninPageResponse)), nil
}

func (h *TenantAdminHandler) PutSigninPage(ctx context.Context, req *connect.Request[anubisv1.PutSigninPageRequest]) (*connect.Response[anubisv1.PutSigninPageResponse], error) {
	if _, err := h.f.Do(ctx, "admin.signin.put", func(ctx context.Context) (any, error) {
		return nil, h.svc.PutSigninPage(ctx, []byte(req.Msg.ConfigJson))
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.PutSigninPageResponse{}), nil
}

// pageProto renders a page for the console, including the URL it is served
// at — the console should not have to know how to build that string.
func (h *TenantAdminHandler) pageProto(p tenancydomain.AuthPage) *anubisv1.AuthPage {
	out := &anubisv1.AuthPage{
		Id: p.ID, Kind: p.Kind, Slug: p.Slug, Name: p.Name, Status: p.Status,
		IsDefault: p.IsDefault, ApplicationId: p.ApplicationID,
		ApplicationSlug: p.ApplicationSlug,
		RealmId:         p.RealmID, RealmCode: p.RealmCode,
		ConfigJson: string(p.Config),
		Url:        h.pageURL(p),
	}
	if !p.CreatedAt.IsZero() {
		out.CreatedAt = p.CreatedAt.Unix()
	}
	if !p.UpdatedAt.IsZero() {
		out.UpdatedAt = p.UpdatedAt.Unix()
	}
	return out
}

func (h *TenantAdminHandler) pageURL(p tenancydomain.AuthPage) string {
	if h.issuer == "" || h.tenantSlug == "" {
		return ""
	}
	return h.issuer + "/p/" + h.tenantSlug + "/" + p.Kind + "/" + p.Slug
}

func (h *TenantAdminHandler) ListAuthPages(ctx context.Context, req *connect.Request[anubisv1.ListAuthPagesRequest]) (*connect.Response[anubisv1.ListAuthPagesResponse], error) {
	out, err := h.f.Do(ctx, "admin.page.list", func(ctx context.Context) (any, error) {
		return h.svc.ListAuthPages(ctx, req.Msg.Kind)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListAuthPagesResponse{}
	for _, p := range out.([]tenancydomain.AuthPage) {
		resp.Pages = append(resp.Pages, h.pageProto(p))
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) GetAuthPage(ctx context.Context, req *connect.Request[anubisv1.GetAuthPageRequest]) (*connect.Response[anubisv1.GetAuthPageResponse], error) {
	out, err := h.f.Do(ctx, "admin.page.get", func(ctx context.Context) (any, error) {
		return h.svc.GetAuthPage(ctx, req.Msg.Id)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.GetAuthPageResponse{
		Page: h.pageProto(*out.(*tenancydomain.AuthPage)),
	}), nil
}

func (h *TenantAdminHandler) CreateAuthPage(ctx context.Context, req *connect.Request[anubisv1.CreateAuthPageRequest]) (*connect.Response[anubisv1.CreateAuthPageResponse], error) {
	out, err := h.f.Do(ctx, "admin.page.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateAuthPage(ctx, pageInput(req.Msg.Page))
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateAuthPageResponse{
		Page: h.pageProto(*out.(*tenancydomain.AuthPage)),
	}), nil
}

func (h *TenantAdminHandler) UpdateAuthPage(ctx context.Context, req *connect.Request[anubisv1.UpdateAuthPageRequest]) (*connect.Response[anubisv1.UpdateAuthPageResponse], error) {
	out, err := h.f.Do(ctx, "admin.page.update", func(ctx context.Context) (any, error) {
		return h.svc.UpdateAuthPage(ctx, pageInput(req.Msg.Page))
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateAuthPageResponse{
		Page: h.pageProto(*out.(*tenancydomain.AuthPage)),
	}), nil
}

func (h *TenantAdminHandler) DeleteAuthPage(ctx context.Context, req *connect.Request[anubisv1.DeleteAuthPageRequest]) (*connect.Response[anubisv1.DeleteAuthPageResponse], error) {
	if _, err := h.f.Do(ctx, "admin.page.delete", func(ctx context.Context) (any, error) {
		return nil, h.svc.DeleteAuthPage(ctx, req.Msg.Id)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.DeleteAuthPageResponse{}), nil
}

func (h *TenantAdminHandler) SetDefaultAuthPage(ctx context.Context, req *connect.Request[anubisv1.SetDefaultAuthPageRequest]) (*connect.Response[anubisv1.SetDefaultAuthPageResponse], error) {
	if _, err := h.f.Do(ctx, "admin.page.set_default", func(ctx context.Context) (any, error) {
		return nil, h.svc.SetDefaultAuthPage(ctx, req.Msg.Id)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SetDefaultAuthPageResponse{}), nil
}

// PreviewAuthPage answers with the validation verdict rather than an error:
// a builder asking "is this draft valid?" is not making a failed request.
func (h *TenantAdminHandler) PreviewAuthPage(ctx context.Context, req *connect.Request[anubisv1.PreviewAuthPageRequest]) (*connect.Response[anubisv1.PreviewAuthPageResponse], error) {
	out, err := h.f.Do(ctx, "admin.page.preview", func(ctx context.Context) (any, error) {
		if perr := h.svc.PreviewAuthPage(ctx, req.Msg.Kind, []byte(req.Msg.ConfigJson)); perr != nil {
			de := apperr.AsError(perr)
			if de.Kind == apperr.KindInvalidArgument {
				return &anubisv1.PreviewAuthPageResponse{Valid: false, Error: de.Error()}, nil
			}
			return nil, perr // permission or internal problems are real errors
		}
		return &anubisv1.PreviewAuthPageResponse{Valid: true}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.PreviewAuthPageResponse)), nil
}

func pageInput(p *anubisv1.AuthPage) tenancydomain.AuthPageInput {
	if p == nil {
		return tenancydomain.AuthPageInput{}
	}
	return tenancydomain.AuthPageInput{
		ID: p.Id, Kind: p.Kind, Slug: p.Slug, Name: p.Name,
		Status: p.Status, ApplicationID: p.ApplicationId,
		RealmID: p.RealmId,
		Config:  []byte(p.ConfigJson),
	}
}

func (h *TenantAdminHandler) UpdateTenant(ctx context.Context,
	req *connect.Request[anubisv1.UpdateTenantRequest],
) (*connect.Response[anubisv1.UpdateTenantResponse], error) {
	if _, err := h.f.Do(ctx, "admin.tenant.update", func(ctx context.Context) (any, error) {
		return nil, h.svc.UpdateTenant(ctx, req.Msg.Id, req.Msg.Name)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateTenantResponse{}), nil
}

func (h *TenantAdminHandler) SetTenantStatus(ctx context.Context,
	req *connect.Request[anubisv1.SetTenantStatusRequest],
) (*connect.Response[anubisv1.SetTenantStatusResponse], error) {
	if _, err := h.f.Do(ctx, "admin.tenant.status", func(ctx context.Context) (any, error) {
		return nil, h.svc.SetTenantStatus(ctx, req.Msg.Id, req.Msg.Status)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SetTenantStatusResponse{}), nil
}

func (h *TenantAdminHandler) GetTenantStats(ctx context.Context,
	req *connect.Request[anubisv1.GetTenantStatsRequest],
) (*connect.Response[anubisv1.GetTenantStatsResponse], error) {
	out, err := h.f.Do(ctx, "admin.tenant.stats", func(ctx context.Context) (any, error) {
		return h.svc.TenantStats(ctx, req.Msg.Id)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	st := out.(*tenancydomain.TenantStats)
	return connect.NewResponse(&anubisv1.GetTenantStatsResponse{
		Identities: int32(st.Identities), Grants: int32(st.Grants),
		ScopeNodes: int32(st.ScopeNodes), Memberships: int32(st.Memberships),
	}), nil
}

func (h *TenantAdminHandler) GetDashboard(ctx context.Context,
	_ *connect.Request[anubisv1.GetDashboardRequest],
) (*connect.Response[anubisv1.GetDashboardResponse], error) {
	out, err := h.f.Do(ctx, "admin.tenant.dashboard", func(ctx context.Context) (any, error) {
		return h.svc.GetDashboard(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	d := out.(*tenancyapp.Dashboard)
	resp := &anubisv1.GetDashboardResponse{
		GrantsTotal:     d.GrantsTotal,
		ScopeNodesTotal: d.ScopeNodesTotal,
		Decisions_24H:   d.Decisions24h,
		Denies_24H:      d.Denies24h,
	}
	for _, r := range d.IdentitiesByRealm {
		resp.IdentitiesByRealm = append(resp.IdentitiesByRealm, &anubisv1.RealmIdentityCount{
			Realm: r.Realm, Kind: r.Kind, Count: r.Count,
		})
	}
	for _, s := range d.Signals {
		resp.Signals = append(resp.Signals, &anubisv1.DashboardSignal{
			Kind: s.Kind, Severity: s.Severity, Count: s.Count,
			Detail: s.Detail, Since: s.Since.Unix(),
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) ListApiKeys(ctx context.Context,
	_ *connect.Request[anubisv1.ListApiKeysRequest],
) (*connect.Response[anubisv1.ListApiKeysResponse], error) {
	out, err := h.f.Do(ctx, "admin.apikey.list", func(ctx context.Context) (any, error) {
		return h.svc.ListAPIKeys(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListApiKeysResponse{}
	for _, k := range out.([]authdomain.APIKeyRecord) {
		pk := &anubisv1.ApiKey{
			Id: k.ID, Label: k.Label, Prefix: k.Lookup,
			CreatedBy: k.CreatedBy, CreatedAt: k.CreatedAt.Unix(),
		}
		if k.LastUsedAt != nil {
			pk.LastUsedAt = k.LastUsedAt.Unix()
		}
		if k.ExpiresAt != nil {
			pk.ExpiresAt = k.ExpiresAt.Unix()
		}
		if k.RevokedAt != nil {
			pk.RevokedAt = k.RevokedAt.Unix()
		}
		resp.Keys = append(resp.Keys, pk)
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) CreateApiKey(ctx context.Context,
	req *connect.Request[anubisv1.CreateApiKeyRequest],
) (*connect.Response[anubisv1.CreateApiKeyResponse], error) {
	out, err := h.f.Do(ctx, "admin.apikey.create", func(ctx context.Context) (any, error) {
		full, prefix, id, cerr := h.svc.CreateAPIKey(ctx, req.Msg.Label, req.Msg.ExpiresAt)
		if cerr != nil {
			return nil, cerr
		}
		return &anubisv1.CreateApiKeyResponse{ApiKey: full, Prefix: prefix, Id: id}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.CreateApiKeyResponse)), nil
}

func (h *TenantAdminHandler) RevokeApiKey(ctx context.Context,
	req *connect.Request[anubisv1.RevokeApiKeyRequest],
) (*connect.Response[anubisv1.RevokeApiKeyResponse], error) {
	if _, err := h.f.Do(ctx, "admin.apikey.revoke", func(ctx context.Context) (any, error) {
		return nil, h.svc.RevokeAPIKey(ctx, req.Msg.Id)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeApiKeyResponse{}), nil
}
