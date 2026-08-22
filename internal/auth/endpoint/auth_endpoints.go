package authep

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"

	"github.com/gsoultan/anubis/internal/auth/app/device"
	"github.com/gsoultan/anubis/internal/auth/app/mfa"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
	authsvc "github.com/gsoultan/anubis/internal/auth/service"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
	"github.com/gsoultan/anubis/internal/platform/mw"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
)

// AuthEndpoints is the wired endpoint set for AuthService.
type AuthEndpoints struct {
	Login           endpoint.Endpoint
	VerifyMfa       endpoint.Endpoint
	Refresh         endpoint.Endpoint
	Logout          endpoint.Endpoint
	LogoutAll       endpoint.Endpoint
	LogoutSession   endpoint.Endpoint
	DeviceChallenge endpoint.Endpoint
	DeviceVerify    endpoint.Endpoint
	Register        endpoint.Endpoint
	VerifyEmail     endpoint.Endpoint
}

// NewAuthEndpoints applies the standard chain everywhere and credential-flow
// rate limits where credentials (or single-use tokens) are consumed.
func NewAuthEndpoints(svc authsvc.AuthService, logger *slog.Logger, limiter *ratelimit.Limiter) AuthEndpoints {
	credLimit := mw.RateLimit(limiter, mw.LoginKeys(30, 10, 600))
	wrap := func(name string, pre []endpoint.Middleware, ep endpoint.Endpoint) endpoint.Endpoint {
		for i := len(pre) - 1; i >= 0; i-- {
			ep = pre[i](ep)
		}
		return mw.Chain(name, logger)(ep)
	}
	none := []endpoint.Middleware{}
	limited := []endpoint.Middleware{credLimit}

	return AuthEndpoints{
		Login: wrap("auth.login", limited, func(ctx context.Context, req any) (any, error) {
			in := req.(loginRateKeys)
			return svc.Login(ctx, in.LoginInput)
		}),
		VerifyMfa: wrap("auth.verify_mfa", limited, func(ctx context.Context, req any) (any, error) {
			return svc.VerifyMfa(ctx, req.(mfa.VerifyMfaInput))
		}),
		Refresh: wrap("auth.refresh", limited, func(ctx context.Context, req any) (any, error) {
			return svc.Refresh(ctx, req.(tokenapp.RefreshInput))
		}),
		Logout: wrap("auth.logout", none, func(ctx context.Context, _ any) (any, error) {
			return nil, svc.Logout(ctx)
		}),
		LogoutAll: wrap("auth.logout_all", none, func(ctx context.Context, _ any) (any, error) {
			n, err := svc.LogoutAll(ctx)
			return n, err
		}),
		LogoutSession: wrap("auth.logout_session", none, func(ctx context.Context, req any) (any, error) {
			return nil, svc.LogoutSession(ctx, req.(string))
		}),
		DeviceChallenge: wrap("auth.device_challenge", limited, func(ctx context.Context, req any) (any, error) {
			return svc.DeviceChallenge(ctx, req.(device.DeviceChallengeInput))
		}),
		DeviceVerify: wrap("auth.device_verify", limited, func(ctx context.Context, req any) (any, error) {
			return svc.DeviceVerify(ctx, req.(device.DeviceVerifyInput))
		}),
		Register: wrap("auth.register", []endpoint.Middleware{mw.RateLimit(limiter, mw.LoginKeys(10, 0, 120))},
			func(ctx context.Context, req any) (any, error) {
				in := req.(registerRateKeys)
				return svc.Register(ctx, in.RegisterInput)
			}),
		VerifyEmail: wrap("auth.verify_email", limited, func(ctx context.Context, req any) (any, error) {
			return nil, svc.VerifyEmail(ctx, req.(string))
		}),
	}
}

// WrapLogin adapts a LoginInput into the rate-key carrier; transports use
// these helpers so the adapters stay unexported.
func WrapLogin(in signin.LoginInput) any             { return loginRateKeys{in} }
func WrapRegister(in registration.RegisterInput) any { return registerRateKeys{in} }
