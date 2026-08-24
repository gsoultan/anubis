package controlrpc

import (
	"context"
	"strconv"
	"strings"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	controlapp "github.com/gsoultan/anubis/internal/control/app"
	"github.com/gsoultan/anubis/internal/platform/mw"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// PlatformAuthHandler is the operators' door.
type PlatformAuthHandler struct {
	uc      controlapp.PlatformAuthUsecase
	f       mw.Factory
	limiter *ratelimit.Limiter
}

func NewPlatformAuthHandler(uc controlapp.PlatformAuthUsecase, f mw.Factory, limiter *ratelimit.Limiter) *PlatformAuthHandler {
	return &PlatformAuthHandler{uc: uc, f: f, limiter: limiter}
}

// platformLoginLimit is deliberately TIGHTER than tenant sign-in.
//
// These accounts run the installation: an operator password is worth more to
// an attacker than any tenant user's, and there are only ever a handful of
// them, so a human hitting this ceiling is doing something unusual. Keyed by
// IP and by account, so guessing one password everywhere and every password
// against one account are both bounded.
const (
	platformLoginPerIP      = 10
	platformLoginPerAccount = 5
	platformLoginBurst      = 5
)

func (h *PlatformAuthHandler) allowLogin(ctx context.Context, username string) error {
	keys := []ratelimit.KeyLimit{}
	if ip := authctx.ClientIP(ctx); ip != "" {
		keys = append(keys, ratelimit.KeyLimit{
			Key:   "platform-login-ip:" + ip,
			Limit: ratelimit.Limit{PerMinute: platformLoginPerIP, Burst: platformLoginBurst},
		})
	}
	if username != "" {
		keys = append(keys, ratelimit.KeyLimit{
			Key:   "platform-login-account:" + strings.ToLower(username),
			Limit: ratelimit.Limit{PerMinute: platformLoginPerAccount, Burst: platformLoginBurst},
		})
	}
	if ok, retry := h.limiter.AllowAll(keys...); !ok {
		return apperr.ErrRateLimited.With("retry_after", strconv.Itoa(int(retry.Seconds())+1))
	}
	return nil
}

var _ anubisv1connect.PlatformAuthServiceHandler = (*PlatformAuthHandler)(nil)

func (h *PlatformAuthHandler) PlatformLogin(ctx context.Context,
	req *connect.Request[anubisv1.PlatformLoginRequest],
) (*connect.Response[anubisv1.PlatformLoginResponse], error) {
	out, err := h.f.Do(ctx, "platform.login", func(ctx context.Context) (any, error) {
		// Before the password is even looked at: an unlimited guessing
		// channel against the accounts that run the installation is the
		// worst thing this endpoint could be.
		if lerr := h.allowLogin(ctx, req.Msg.Username); lerr != nil {
			return nil, lerr
		}
		return h.uc.Login(ctx, req.Msg.Username, req.Msg.Password)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	s := out.(*controlapp.PlatformSession)
	return connect.NewResponse(&anubisv1.PlatformLoginResponse{
		AccessToken: s.AccessToken,
		ExpiresIn:   int32(s.ExpiresIn),
		Username:    s.Username,
		// Set when a second factor is required; AccessToken is empty then.
		MfaToken: s.MFAToken,
		Owner:    s.Owner,
	}), nil
}

func (h *PlatformAuthHandler) MyTenants(ctx context.Context,
	_ *connect.Request[anubisv1.MyTenantsRequest],
) (*connect.Response[anubisv1.MyTenantsResponse], error) {
	out, err := h.f.Do(ctx, "platform.my_tenants", func(ctx context.Context) (any, error) {
		return h.uc.MyTenants(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.MyTenantsResponse{}
	for _, t := range out.([]controlapp.TenantChoice) {
		resp.Tenants = append(resp.Tenants, &anubisv1.MyTenant{
			Slug: t.Slug, Name: t.Name, Role: t.Role, All: t.All,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *PlatformAuthHandler) PlatformVerifyMfa(ctx context.Context,
	req *connect.Request[anubisv1.PlatformVerifyMfaRequest],
) (*connect.Response[anubisv1.PlatformVerifyMfaResponse], error) {
	out, err := h.f.Do(ctx, "platform.verify_mfa", func(ctx context.Context) (any, error) {
		// Limited on the same keys as the password step: a code is six
		// digits, and an unbounded channel guesses it in minutes.
		if lerr := h.allowLogin(ctx, ""); lerr != nil {
			return nil, lerr
		}
		return h.uc.VerifyMFA(ctx, req.Msg.MfaToken, req.Msg.Code)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	s := out.(*controlapp.PlatformSession)
	return connect.NewResponse(&anubisv1.PlatformVerifyMfaResponse{
		AccessToken: s.AccessToken, ExpiresIn: int32(s.ExpiresIn),
		Username: s.Username, Owner: s.Owner,
	}), nil
}

func (h *PlatformAuthHandler) BeginTotpEnrolment(ctx context.Context,
	_ *connect.Request[anubisv1.BeginTotpEnrolmentRequest],
) (*connect.Response[anubisv1.BeginTotpEnrolmentResponse], error) {
	type pair struct{ secret, uri string }
	out, err := h.f.Do(ctx, "platform.totp_begin", func(ctx context.Context) (any, error) {
		sec, uri, berr := h.uc.BeginTOTPEnrolment(ctx)
		return pair{secret: sec, uri: uri}, berr
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	p := out.(pair)
	return connect.NewResponse(&anubisv1.BeginTotpEnrolmentResponse{Secret: p.secret, Uri: p.uri}), nil
}

func (h *PlatformAuthHandler) ConfirmTotpEnrolment(ctx context.Context,
	req *connect.Request[anubisv1.ConfirmTotpEnrolmentRequest],
) (*connect.Response[anubisv1.ConfirmTotpEnrolmentResponse], error) {
	if _, err := h.f.Do(ctx, "platform.totp_confirm", func(ctx context.Context) (any, error) {
		return nil, h.uc.ConfirmTOTPEnrolment(ctx, req.Msg.Code)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ConfirmTotpEnrolmentResponse{}), nil
}
