package identityrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identitysvc "github.com/gsoultan/anubis/internal/identity/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// IdentityAdminHandler implements anubisv1connect.IdentityAdminServiceHandler.
type IdentityAdminHandler struct {
	svc identitysvc.IdentityAdminService
	f   mw.Factory
}

func NewIdentityAdminHandler(svc identitysvc.IdentityAdminService, f mw.Factory) *IdentityAdminHandler {
	return &IdentityAdminHandler{svc: svc, f: f}
}

var _ anubisv1connect.IdentityAdminServiceHandler = (*IdentityAdminHandler)(nil)

func identityProto(r *identitydomain.IdentityRecord) *anubisv1.Identity {
	out := &anubisv1.Identity{
		Id: r.ID, Username: r.Username, Email: r.Email,
		Realm: r.RealmCode, RealmKind: r.RealmKind, Status: r.Status,
		Category: r.Category, ExternalRef: r.ExternalRef,
		AssuranceLevel: int32(r.AssuranceLevel), TokenEpoch: int32(r.TokenEpoch),
		CreatedAt: r.CreatedAt.Unix(),
	}
	if r.LastLoginAt != nil {
		out.LastLoginAt = r.LastLoginAt.Unix()
	}
	if r.DisabledAt != nil {
		out.DisabledAt = r.DisabledAt.Unix()
	}
	if r.AnonymizedAt != nil {
		out.AnonymizedAt = r.AnonymizedAt.Unix()
	}
	return out
}

func credentialProto(c credential.CredentialInfo) *anubisv1.CredentialInfo {
	out := &anubisv1.CredentialInfo{
		Id: c.ID, Kind: c.Kind, Label: c.Label, LookupKey: c.LookupKey,
		CreatedAt: c.CreatedAt.Unix(),
	}
	if c.LastUsedAt != nil {
		out.LastUsedAt = c.LastUsedAt.Unix()
	}
	if c.ExpiresAt != nil {
		out.ExpiresAt = c.ExpiresAt.Unix()
	}
	if c.RevokedAt != nil {
		out.RevokedAt = c.RevokedAt.Unix()
	}
	return out
}

func (h *IdentityAdminHandler) ListIdentities(ctx context.Context, req *connect.Request[anubisv1.ListIdentitiesRequest]) (*connect.Response[anubisv1.ListIdentitiesResponse], error) {
	out, err := h.f.Do(ctx, "admin.identity.list", func(ctx context.Context) (any, error) {
		list, next, err := h.svc.ListIdentities(ctx, identitydomain.IdentityFilter{
			RealmID: req.Msg.Realm,
			Status:  req.Msg.Status, Query: req.Msg.Query,
			AfterID: req.Msg.PageToken, Limit: int(req.Msg.PageSize),
		})
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListIdentitiesResponse{NextPageToken: next}
		for i := range list {
			resp.Identities = append(resp.Identities, identityProto(&list[i]))
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListIdentitiesResponse)), nil
}

func (h *IdentityAdminHandler) GetIdentity(ctx context.Context, req *connect.Request[anubisv1.GetIdentityRequest]) (*connect.Response[anubisv1.GetIdentityResponse], error) {
	out, err := h.f.Do(ctx, "admin.identity.get", func(ctx context.Context) (any, error) {
		rec, creds, err := h.svc.GetIdentity(ctx, req.Msg.Id)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.GetIdentityResponse{Identity: identityProto(rec)}
		for _, c := range creds {
			resp.Credentials = append(resp.Credentials, credentialProto(c))
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.GetIdentityResponse)), nil
}

func (h *IdentityAdminHandler) CreateIdentity(ctx context.Context, req *connect.Request[anubisv1.CreateIdentityRequest]) (*connect.Response[anubisv1.CreateIdentityResponse], error) {
	out, err := h.f.Do(ctx, "admin.identity.create", func(ctx context.Context) (any, error) {
		rec, err := h.svc.CreateIdentity(ctx, identityapp.AdminCreateIdentity{
			Realm: req.Msg.Realm, Username: req.Msg.Username, Email: req.Msg.Email,
			Password: req.Msg.Password, Category: req.Msg.Category,
			ExternalRef: req.Msg.ExternalRef, AssuranceLevel: int(req.Msg.AssuranceLevel),
		})
		if err != nil {
			return nil, err
		}
		return &anubisv1.CreateIdentityResponse{Identity: identityProto(rec)}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.CreateIdentityResponse)), nil
}

func (h *IdentityAdminHandler) DisableIdentity(ctx context.Context, req *connect.Request[anubisv1.DisableIdentityRequest]) (*connect.Response[anubisv1.DisableIdentityResponse], error) {
	if _, err := h.f.Do(ctx, "admin.identity.disable", func(ctx context.Context) (any, error) {
		return nil, h.svc.DisableIdentity(ctx, req.Msg.Id, req.Msg.Reason)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.DisableIdentityResponse{}), nil
}

func (h *IdentityAdminHandler) EnableIdentity(ctx context.Context, req *connect.Request[anubisv1.EnableIdentityRequest]) (*connect.Response[anubisv1.EnableIdentityResponse], error) {
	if _, err := h.f.Do(ctx, "admin.identity.enable", func(ctx context.Context) (any, error) {
		return nil, h.svc.EnableIdentity(ctx, req.Msg.Id)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.EnableIdentityResponse{}), nil
}

func (h *IdentityAdminHandler) BumpTokenEpoch(ctx context.Context, req *connect.Request[anubisv1.BumpTokenEpochRequest]) (*connect.Response[anubisv1.BumpTokenEpochResponse], error) {
	out, err := h.f.Do(ctx, "admin.identity.epoch", func(ctx context.Context) (any, error) {
		return h.svc.BumpTokenEpoch(ctx, req.Msg.Id)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.BumpTokenEpochResponse{TokenEpoch: int32(out.(int))}), nil
}

func (h *IdentityAdminHandler) SetPassword(ctx context.Context, req *connect.Request[anubisv1.SetPasswordRequest]) (*connect.Response[anubisv1.SetPasswordResponse], error) {
	if _, err := h.f.Do(ctx, "admin.identity.set_password", func(ctx context.Context) (any, error) {
		return nil, h.svc.SetPassword(ctx, req.Msg.Id, req.Msg.Password)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SetPasswordResponse{}), nil
}

func (h *IdentityAdminHandler) LinkIdentities(ctx context.Context, req *connect.Request[anubisv1.LinkIdentitiesRequest]) (*connect.Response[anubisv1.LinkIdentitiesResponse], error) {
	if _, err := h.f.Do(ctx, "admin.identity.link", func(ctx context.Context) (any, error) {
		return nil, h.svc.LinkIdentities(ctx, req.Msg.PrimaryId, req.Msg.SecondaryId,
			req.Msg.Method, req.Msg.EvidenceJson)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LinkIdentitiesResponse{}), nil
}

func (h *IdentityAdminHandler) RequestErasure(ctx context.Context, req *connect.Request[anubisv1.RequestErasureRequest]) (*connect.Response[anubisv1.RequestErasureResponse], error) {
	if _, err := h.f.Do(ctx, "admin.identity.erasure", func(ctx context.Context) (any, error) {
		return nil, h.svc.RequestErasure(ctx, req.Msg.Id)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RequestErasureResponse{}), nil
}

func (h *IdentityAdminHandler) ListCredentials(ctx context.Context, req *connect.Request[anubisv1.ListCredentialsRequest]) (*connect.Response[anubisv1.ListCredentialsResponse], error) {
	out, err := h.f.Do(ctx, "admin.credential.list", func(ctx context.Context) (any, error) {
		return h.svc.ListCredentials(ctx, req.Msg.IdentityId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListCredentialsResponse{}
	for _, c := range out.([]credential.CredentialInfo) {
		resp.Credentials = append(resp.Credentials, credentialProto(c))
	}
	return connect.NewResponse(resp), nil
}

func (h *IdentityAdminHandler) RevokeCredential(ctx context.Context, req *connect.Request[anubisv1.RevokeCredentialRequest]) (*connect.Response[anubisv1.RevokeCredentialResponse], error) {
	if _, err := h.f.Do(ctx, "admin.credential.revoke", func(ctx context.Context) (any, error) {
		return nil, h.svc.RevokeCredential(ctx, req.Msg.CredentialId)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeCredentialResponse{}), nil
}

func consentProto(c identitydomain.ConsentRecord) *anubisv1.Consent {
	out := &anubisv1.Consent{
		Id: c.ID, Purpose: c.Purpose, PolicyVersion: c.PolicyVersion,
		GrantedAt: c.GrantedAt.Unix(),
	}
	if c.WithdrawnAt != nil {
		out.WithdrawnAt = c.WithdrawnAt.Unix()
	}
	if c.ExpiresAt != nil {
		out.ExpiresAt = c.ExpiresAt.Unix()
	}
	return out
}

func (h *IdentityAdminHandler) ListConsents(ctx context.Context, req *connect.Request[anubisv1.ListConsentsRequest]) (*connect.Response[anubisv1.ListConsentsResponse], error) {
	out, err := h.f.Do(ctx, "admin.consent.list", func(ctx context.Context) (any, error) {
		return h.svc.ListConsents(ctx, req.Msg.IdentityId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListConsentsResponse{}
	for _, c := range out.([]identitydomain.ConsentRecord) {
		resp.Consents = append(resp.Consents, consentProto(c))
	}
	return connect.NewResponse(resp), nil
}

func (h *IdentityAdminHandler) RecordConsent(ctx context.Context, req *connect.Request[anubisv1.RecordConsentRequest]) (*connect.Response[anubisv1.RecordConsentResponse], error) {
	out, err := h.f.Do(ctx, "admin.consent.record", func(ctx context.Context) (any, error) {
		rec, err := h.svc.RecordConsent(ctx, req.Msg.IdentityId, req.Msg.Purpose,
			req.Msg.PolicyVersion, req.Msg.EvidenceJson)
		if err != nil {
			return nil, err
		}
		return &anubisv1.RecordConsentResponse{Consent: consentProto(*rec)}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.RecordConsentResponse)), nil
}

func (h *IdentityAdminHandler) WithdrawConsent(ctx context.Context, req *connect.Request[anubisv1.WithdrawConsentRequest]) (*connect.Response[anubisv1.WithdrawConsentResponse], error) {
	if _, err := h.f.Do(ctx, "admin.consent.withdraw", func(ctx context.Context) (any, error) {
		return nil, h.svc.WithdrawConsent(ctx, req.Msg.ConsentId)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.WithdrawConsentResponse{}), nil
}

func (h *IdentityAdminHandler) GetIdentityAttributes(ctx context.Context, req *connect.Request[anubisv1.GetIdentityAttributesRequest]) (*connect.Response[anubisv1.GetIdentityAttributesResponse], error) {
	out, err := h.f.Do(ctx, "admin.identity.attributes_get", func(ctx context.Context) (any, error) {
		attrs, erased, err := h.svc.IdentityAttributes(ctx, req.Msg.Id)
		if err != nil {
			return nil, err
		}
		return &anubisv1.GetIdentityAttributesResponse{Attributes: attrs, Erased: erased}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.GetIdentityAttributesResponse)), nil
}

func (h *IdentityAdminHandler) SetIdentityAttributes(ctx context.Context, req *connect.Request[anubisv1.SetIdentityAttributesRequest]) (*connect.Response[anubisv1.SetIdentityAttributesResponse], error) {
	if _, err := h.f.Do(ctx, "admin.identity.attributes_set", func(ctx context.Context) (any, error) {
		return nil, h.svc.SetIdentityAttributes(ctx, req.Msg.Id, req.Msg.Attributes)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SetIdentityAttributesResponse{}), nil
}
