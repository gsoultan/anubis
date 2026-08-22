package authsvc

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	"github.com/gsoultan/anubis/internal/auth/app/device"
	"github.com/gsoultan/anubis/internal/auth/app/mfa"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
)

// AuthService is the authentication surface (proto AuthService).
type AuthService interface {
	Login(ctx context.Context, in signin.LoginInput) (*signin.LoginOutput, error)
	VerifyMfa(ctx context.Context, in mfa.VerifyMfaInput) (*authapp.TokenPair, error)
	Refresh(ctx context.Context, in tokenapp.RefreshInput) (*authapp.TokenPair, error)
	Logout(ctx context.Context) error
	LogoutAll(ctx context.Context) (int, error)
	LogoutSession(ctx context.Context, sessionID string) error
	DeviceChallenge(ctx context.Context, in device.DeviceChallengeInput) (*device.DeviceChallengeOutput, error)
	DeviceVerify(ctx context.Context, in device.DeviceVerifyInput) (*authapp.TokenPair, error)
	Register(ctx context.Context, in registration.RegisterInput) (*registration.RegisterOutput, error)
	VerifyEmail(ctx context.Context, token string) error
}
