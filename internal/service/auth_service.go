package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/usecase"
)

// AuthService is the authentication surface (proto AuthService).
type AuthService interface {
	Login(ctx context.Context, in usecase.LoginInput) (*usecase.LoginOutput, error)
	VerifyMfa(ctx context.Context, in usecase.VerifyMfaInput) (*usecase.TokenPair, error)
	Refresh(ctx context.Context, in usecase.RefreshInput) (*usecase.TokenPair, error)
	Logout(ctx context.Context) error
	LogoutAll(ctx context.Context) (int, error)
	LogoutSession(ctx context.Context, sessionID string) error
	DeviceChallenge(ctx context.Context, in usecase.DeviceChallengeInput) (*usecase.DeviceChallengeOutput, error)
	DeviceVerify(ctx context.Context, in usecase.DeviceVerifyInput) (*usecase.TokenPair, error)
	Register(ctx context.Context, in usecase.RegisterInput) (*usecase.RegisterOutput, error)
	VerifyEmail(ctx context.Context, token string) error
}
