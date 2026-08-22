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
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancysvc "github.com/gsoultan/anubis/internal/tenancy/service"
)

// TenantAdminHandler implements anubisv1connect.TenantAdminServiceHandler.
type TenantAdminHandler struct {
	svc tenancysvc.TenantAdminService
	f   mw.Factory
}

func NewTenantAdminHandler(svc tenancysvc.TenantAdminService, f mw.Factory) *TenantAdminHandler {
	return &TenantAdminHandler{svc: svc, f: f}
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
		PasswordPolicyJson: string(r.PasswordPolicy),
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
		PasswordPolicy: []byte(r.PasswordPolicyJson),
	}
}

func appProto(a *tenancydomain.ApplicationRecord) *anubisv1.Application {
	return &anubisv1.Application{
		Id: a.ID, Slug: a.Slug, Name: a.Name, Kind: a.Kind, Status: a.Status,
		RedirectUris: a.RedirectURIs, BackchannelLogoutUri: a.BackchannelLogoutURI,
		TokenFormat: a.TokenFormat, AccessTokenTtl: a.AccessTokenTTL,
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

func (h *TenantAdminHandler) ListApplications(ctx context.Context, _ *connect.Request[anubisv1.ListApplicationsRequest]) (*connect.Response[anubisv1.ListApplicationsResponse], error) {
	out, err := h.f.Do(ctx, "admin.application.list", func(ctx context.Context) (any, error) {
		return h.svc.ListApplications(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListApplicationsResponse{}
	for i := range out.([]tenancydomain.ApplicationRecord) {
		a := out.([]tenancydomain.ApplicationRecord)[i]
		resp.Applications = append(resp.Applications, appProto(&a))
	}
	return connect.NewResponse(resp), nil
}

func (h *TenantAdminHandler) CreateApplication(ctx context.Context, req *connect.Request[anubisv1.CreateApplicationRequest]) (*connect.Response[anubisv1.CreateApplicationResponse], error) {
	out, err := h.f.Do(ctx, "admin.application.create", func(ctx context.Context) (any, error) {
		a := req.Msg.Application
		rec, secret, err := h.svc.CreateApplication(ctx, tenancydomain.ApplicationRecord{
			Slug: a.Slug, Name: a.Name, Kind: a.Kind,
			RedirectURIs: a.RedirectUris, BackchannelLogoutURI: a.BackchannelLogoutUri,
			TokenFormat: a.TokenFormat, AccessTokenTTL: a.AccessTokenTtl,
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
			RedirectURIs: a.RedirectUris, BackchannelLogoutURI: a.BackchannelLogoutUri,
			TokenFormat: a.TokenFormat, AccessTokenTTL: a.AccessTokenTtl,
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
