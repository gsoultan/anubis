package connectrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	ep "github.com/gsoultan/anubis/internal/endpoint"
	"github.com/gsoultan/anubis/internal/usecase"
)

// AuthHandler implements anubisv1connect.AuthServiceHandler over the
// endpoint set.
type AuthHandler struct {
	eps ep.AuthEndpoints
}

func NewAuthHandler(eps ep.AuthEndpoints) *AuthHandler { return &AuthHandler{eps: eps} }

var _ anubisv1connect.AuthServiceHandler = (*AuthHandler)(nil)

func pair(p *usecase.TokenPair) *anubisv1.TokenPair {
	if p == nil {
		return nil
	}
	return &anubisv1.TokenPair{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		TokenType:    p.TokenType,
		ExpiresIn:    int32(p.ExpiresIn),
		SessionId:    p.SessionID,
	}
}

func (h *AuthHandler) Login(ctx context.Context, req *connect.Request[anubisv1.LoginRequest]) (*connect.Response[anubisv1.LoginResponse], error) {
	out, err := h.eps.Login(ctx, ep.WrapLogin(usecase.LoginInput{
		Tenant:   req.Msg.Tenant,
		Realm:    req.Msg.Realm,
		Username: req.Msg.Username,
		Password: req.Msg.Password,
		ClientID: req.Msg.ClientId,
		DeviceFP: req.Msg.DeviceFp,
	}))
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	lo := out.(*usecase.LoginOutput)
	resp := &anubisv1.LoginResponse{}
	if lo.MFA != nil {
		resp.Result = &anubisv1.LoginResponse_Mfa{Mfa: &anubisv1.MfaChallenge{
			MfaToken:  lo.MFA.MFAToken,
			Methods:   lo.MFA.Methods,
			ExpiresIn: int32(lo.MFA.ExpiresIn),
		}}
	} else {
		resp.Result = &anubisv1.LoginResponse_Tokens{Tokens: pair(lo.Tokens)}
	}
	return connect.NewResponse(resp), nil
}

func (h *AuthHandler) VerifyMfa(ctx context.Context, req *connect.Request[anubisv1.VerifyMfaRequest]) (*connect.Response[anubisv1.VerifyMfaResponse], error) {
	out, err := h.eps.VerifyMfa(ctx, usecase.VerifyMfaInput{
		MFAToken: req.Msg.MfaToken, Method: req.Msg.Method, Code: req.Msg.Code,
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.VerifyMfaResponse{
		Tokens: pair(out.(*usecase.TokenPair)),
	}), nil
}

func (h *AuthHandler) Refresh(ctx context.Context, req *connect.Request[anubisv1.RefreshRequest]) (*connect.Response[anubisv1.RefreshResponse], error) {
	out, err := h.eps.Refresh(ctx, usecase.RefreshInput{RefreshToken: req.Msg.RefreshToken})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RefreshResponse{
		Tokens: pair(out.(*usecase.TokenPair)),
	}), nil
}

func (h *AuthHandler) Logout(ctx context.Context, _ *connect.Request[anubisv1.LogoutRequest]) (*connect.Response[anubisv1.LogoutResponse], error) {
	if _, err := h.eps.Logout(ctx, nil); err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LogoutResponse{}), nil
}

func (h *AuthHandler) LogoutAll(ctx context.Context, _ *connect.Request[anubisv1.LogoutAllRequest]) (*connect.Response[anubisv1.LogoutAllResponse], error) {
	out, err := h.eps.LogoutAll(ctx, nil)
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LogoutAllResponse{
		SessionsRevoked: int32(out.(int)),
	}), nil
}

func (h *AuthHandler) LogoutSession(ctx context.Context, req *connect.Request[anubisv1.LogoutSessionRequest]) (*connect.Response[anubisv1.LogoutSessionResponse], error) {
	if _, err := h.eps.LogoutSession(ctx, req.Msg.SessionId); err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LogoutSessionResponse{}), nil
}

func (h *AuthHandler) DeviceChallenge(ctx context.Context, req *connect.Request[anubisv1.DeviceChallengeRequest]) (*connect.Response[anubisv1.DeviceChallengeResponse], error) {
	out, err := h.eps.DeviceChallenge(ctx, usecase.DeviceChallengeInput{
		Tenant: req.Msg.Tenant, Realm: req.Msg.Realm, DeviceID: req.Msg.DeviceId,
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	ch := out.(*usecase.DeviceChallengeOutput)
	return connect.NewResponse(&anubisv1.DeviceChallengeResponse{
		Nonce: ch.Nonce, ExpiresIn: int32(ch.ExpiresIn),
	}), nil
}

func (h *AuthHandler) DeviceVerify(ctx context.Context, req *connect.Request[anubisv1.DeviceVerifyRequest]) (*connect.Response[anubisv1.DeviceVerifyResponse], error) {
	out, err := h.eps.DeviceVerify(ctx, usecase.DeviceVerifyInput{
		Tenant: req.Msg.Tenant, Nonce: req.Msg.Nonce, KeyID: req.Msg.KeyId,
		Signature: req.Msg.Signature, ClientID: req.Msg.ClientId, DeviceFP: req.Msg.DeviceFp,
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.DeviceVerifyResponse{
		Tokens: pair(out.(*usecase.TokenPair)),
	}), nil
}

func (h *AuthHandler) Register(ctx context.Context, req *connect.Request[anubisv1.RegisterRequest]) (*connect.Response[anubisv1.RegisterResponse], error) {
	in := usecase.RegisterInput{
		Tenant: req.Msg.Tenant, Realm: req.Msg.Realm,
		Username: req.Msg.Username, Email: req.Msg.Email, Password: req.Msg.Password,
	}
	for _, c := range req.Msg.Consents {
		in.Consents = append(in.Consents, usecase.RegisterConsent{
			Purpose: c.Purpose, PolicyVersion: c.PolicyVersion,
		})
	}
	out, err := h.eps.Register(ctx, ep.WrapRegister(in))
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	ro := out.(*usecase.RegisterOutput)
	return connect.NewResponse(&anubisv1.RegisterResponse{
		IdentityId: ro.IdentityID, VerificationRequired: ro.VerificationRequired,
	}), nil
}

func (h *AuthHandler) VerifyEmail(ctx context.Context, req *connect.Request[anubisv1.VerifyEmailRequest]) (*connect.Response[anubisv1.VerifyEmailResponse], error) {
	if _, err := h.eps.VerifyEmail(ctx, req.Msg.Token); err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.VerifyEmailResponse{}), nil
}
