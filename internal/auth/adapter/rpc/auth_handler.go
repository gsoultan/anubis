package authrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	"github.com/gsoultan/anubis/internal/auth/app/clientcreds"
	"github.com/gsoultan/anubis/internal/auth/app/device"
	"github.com/gsoultan/anubis/internal/auth/app/enroll"
	"github.com/gsoultan/anubis/internal/auth/app/mfa"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
	authep "github.com/gsoultan/anubis/internal/auth/endpoint"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
)

// AuthHandler implements anubisv1connect.AuthServiceHandler over the
// endpoint set.
type AuthHandler struct {
	eps authep.AuthEndpoints
}

func NewAuthHandler(eps authep.AuthEndpoints) *AuthHandler { return &AuthHandler{eps: eps} }

var _ anubisv1connect.AuthServiceHandler = (*AuthHandler)(nil)

// TokenPairProto renders an issued pair on the wire. Exported because
// scope switching (authz context) returns one too.
func TokenPairProto(p *authapp.TokenPair) *anubisv1.TokenPair {
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
	out, err := h.eps.Login(ctx, authep.WrapLogin(signin.LoginInput{
		Tenant:   req.Msg.Tenant,
		Realm:    req.Msg.Realm,
		Username: req.Msg.Username,
		Password: req.Msg.Password,
		ClientID: req.Msg.ClientId,
		DeviceFP: req.Msg.DeviceFp,
	}))
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	lo := out.(*signin.LoginOutput)
	resp := &anubisv1.LoginResponse{}
	if lo.MFA != nil {
		resp.Result = &anubisv1.LoginResponse_Mfa{Mfa: &anubisv1.MfaChallenge{
			MfaToken:  lo.MFA.MFAToken,
			Methods:   lo.MFA.Methods,
			ExpiresIn: int32(lo.MFA.ExpiresIn),
		}}
	} else {
		resp.Result = &anubisv1.LoginResponse_Tokens{Tokens: TokenPairProto(lo.Tokens)}
	}
	return connect.NewResponse(resp), nil
}

func (h *AuthHandler) VerifyMfa(ctx context.Context, req *connect.Request[anubisv1.VerifyMfaRequest]) (*connect.Response[anubisv1.VerifyMfaResponse], error) {
	out, err := h.eps.VerifyMfa(ctx, mfa.VerifyMfaInput{
		MFAToken: req.Msg.MfaToken, Method: req.Msg.Method, Code: req.Msg.Code,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.VerifyMfaResponse{
		Tokens: TokenPairProto(out.(*authapp.TokenPair)),
	}), nil
}

func (h *AuthHandler) Refresh(ctx context.Context, req *connect.Request[anubisv1.RefreshRequest]) (*connect.Response[anubisv1.RefreshResponse], error) {
	out, err := h.eps.Refresh(ctx, tokenapp.RefreshInput{RefreshToken: req.Msg.RefreshToken})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RefreshResponse{
		Tokens: TokenPairProto(out.(*authapp.TokenPair)),
	}), nil
}

func (h *AuthHandler) Logout(ctx context.Context, _ *connect.Request[anubisv1.LogoutRequest]) (*connect.Response[anubisv1.LogoutResponse], error) {
	if _, err := h.eps.Logout(ctx, nil); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LogoutResponse{}), nil
}

func (h *AuthHandler) LogoutAll(ctx context.Context, _ *connect.Request[anubisv1.LogoutAllRequest]) (*connect.Response[anubisv1.LogoutAllResponse], error) {
	out, err := h.eps.LogoutAll(ctx, nil)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LogoutAllResponse{
		SessionsRevoked: int32(out.(int)),
	}), nil
}

func (h *AuthHandler) LogoutSession(ctx context.Context, req *connect.Request[anubisv1.LogoutSessionRequest]) (*connect.Response[anubisv1.LogoutSessionResponse], error) {
	if _, err := h.eps.LogoutSession(ctx, req.Msg.SessionId); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.LogoutSessionResponse{}), nil
}

func (h *AuthHandler) DeviceChallenge(ctx context.Context, req *connect.Request[anubisv1.DeviceChallengeRequest]) (*connect.Response[anubisv1.DeviceChallengeResponse], error) {
	out, err := h.eps.DeviceChallenge(ctx, device.DeviceChallengeInput{
		Tenant: req.Msg.Tenant, Realm: req.Msg.Realm, DeviceID: req.Msg.DeviceId,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	ch := out.(*device.DeviceChallengeOutput)
	return connect.NewResponse(&anubisv1.DeviceChallengeResponse{
		Nonce: ch.Nonce, ExpiresIn: int32(ch.ExpiresIn),
	}), nil
}

func (h *AuthHandler) DeviceVerify(ctx context.Context, req *connect.Request[anubisv1.DeviceVerifyRequest]) (*connect.Response[anubisv1.DeviceVerifyResponse], error) {
	out, err := h.eps.DeviceVerify(ctx, device.DeviceVerifyInput{
		Tenant: req.Msg.Tenant, Nonce: req.Msg.Nonce, KeyID: req.Msg.KeyId,
		Signature: req.Msg.Signature, ClientID: req.Msg.ClientId, DeviceFP: req.Msg.DeviceFp,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.DeviceVerifyResponse{
		Tokens: TokenPairProto(out.(*authapp.TokenPair)),
	}), nil
}

func (h *AuthHandler) Register(ctx context.Context, req *connect.Request[anubisv1.RegisterRequest]) (*connect.Response[anubisv1.RegisterResponse], error) {
	in := registration.RegisterInput{
		Tenant: req.Msg.Tenant, Realm: req.Msg.Realm,
		Username: req.Msg.Username, Email: req.Msg.Email, Password: req.Msg.Password,
	}
	for _, c := range req.Msg.Consents {
		in.Consents = append(in.Consents, registration.RegisterConsent{
			Purpose: c.Purpose, PolicyVersion: c.PolicyVersion,
		})
	}
	out, err := h.eps.Register(ctx, authep.WrapRegister(in))
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	ro := out.(*registration.RegisterOutput)
	return connect.NewResponse(&anubisv1.RegisterResponse{
		IdentityId: ro.IdentityID, VerificationRequired: ro.VerificationRequired,
	}), nil
}

func (h *AuthHandler) VerifyEmail(ctx context.Context, req *connect.Request[anubisv1.VerifyEmailRequest]) (*connect.Response[anubisv1.VerifyEmailResponse], error) {
	if _, err := h.eps.VerifyEmail(ctx, req.Msg.Token); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.VerifyEmailResponse{}), nil
}

func (h *AuthHandler) BeginTotpEnrollment(ctx context.Context, _ *connect.Request[anubisv1.BeginTotpEnrollmentRequest]) (*connect.Response[anubisv1.BeginTotpEnrollmentResponse], error) {
	out, err := h.eps.BeginTotp(ctx, nil)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	e := out.(*enroll.TOTPEnrollment)
	return connect.NewResponse(&anubisv1.BeginTotpEnrollmentResponse{
		ProvisioningUri: e.ProvisioningURI, Secret: e.Secret,
		EnrollmentToken: e.EnrollmentToken, ExpiresIn: int32(e.ExpiresIn),
	}), nil
}

func (h *AuthHandler) ConfirmTotpEnrollment(ctx context.Context, req *connect.Request[anubisv1.ConfirmTotpEnrollmentRequest]) (*connect.Response[anubisv1.ConfirmTotpEnrollmentResponse], error) {
	out, err := h.eps.ConfirmTotp(ctx, [2]string{req.Msg.EnrollmentToken, req.Msg.Code})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	c := out.(*enroll.TOTPConfirmation)
	return connect.NewResponse(&anubisv1.ConfirmTotpEnrollmentResponse{
		CredentialId: c.CredentialID, RecoveryCodes: c.RecoveryCodes,
	}), nil
}

func (h *AuthHandler) EnrollDeviceKey(ctx context.Context, req *connect.Request[anubisv1.EnrollDeviceKeyRequest]) (*connect.Response[anubisv1.EnrollDeviceKeyResponse], error) {
	out, err := h.eps.EnrollDeviceKey(ctx, [2]string{req.Msg.PublicKey, req.Msg.Label})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.EnrollDeviceKeyResponse{CredentialId: out.(string)}), nil
}

func (h *AuthHandler) ClientCredentials(ctx context.Context, req *connect.Request[anubisv1.ClientCredentialsRequest]) (*connect.Response[anubisv1.ClientCredentialsResponse], error) {
	out, err := h.eps.ClientCreds(ctx, authep.WrapClientCredentials(clientcreds.ClientCredentialsInput{
		Tenant: req.Msg.Tenant, ClientID: req.Msg.ClientId,
		ClientSecret: req.Msg.ClientSecret, Audience: req.Msg.Audience,
	}))
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	c := out.(*clientcreds.ClientCredentialsOutput)
	return connect.NewResponse(&anubisv1.ClientCredentialsResponse{
		AccessToken: c.AccessToken, TokenType: c.TokenType, ExpiresIn: int32(c.ExpiresIn),
	}), nil
}
