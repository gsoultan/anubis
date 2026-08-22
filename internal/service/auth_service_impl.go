package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/usecase"
)

// authService delegates each method to its usecase.
type authService struct {
	login         usecase.LoginUsecase
	verifyMfa     usecase.VerifyMfaUsecase
	refresh       usecase.RefreshUsecase
	logout        usecase.LogoutUsecase
	logoutAll     usecase.LogoutAllUsecase
	logoutSession usecase.LogoutSessionUsecase
	deviceCh      usecase.DeviceChallengeUsecase
	deviceVerify  usecase.DeviceVerifyUsecase
	register      usecase.RegisterUsecase
	verifyEmail   usecase.VerifyEmailUsecase
}

func NewAuthService(
	login usecase.LoginUsecase,
	verifyMfa usecase.VerifyMfaUsecase,
	refresh usecase.RefreshUsecase,
	logout usecase.LogoutUsecase,
	logoutAll usecase.LogoutAllUsecase,
	logoutSession usecase.LogoutSessionUsecase,
	deviceCh usecase.DeviceChallengeUsecase,
	deviceVerify usecase.DeviceVerifyUsecase,
	register usecase.RegisterUsecase,
	verifyEmail usecase.VerifyEmailUsecase,
) AuthService {
	return &authService{
		login: login, verifyMfa: verifyMfa, refresh: refresh,
		logout: logout, logoutAll: logoutAll, logoutSession: logoutSession,
		deviceCh: deviceCh, deviceVerify: deviceVerify,
		register: register, verifyEmail: verifyEmail,
	}
}

func (s *authService) Login(ctx context.Context, in usecase.LoginInput) (*usecase.LoginOutput, error) {
	return s.login.Execute(ctx, in)
}

func (s *authService) VerifyMfa(ctx context.Context, in usecase.VerifyMfaInput) (*usecase.TokenPair, error) {
	return s.verifyMfa.Execute(ctx, in)
}

func (s *authService) Refresh(ctx context.Context, in usecase.RefreshInput) (*usecase.TokenPair, error) {
	return s.refresh.Execute(ctx, in)
}

func (s *authService) Logout(ctx context.Context) error { return s.logout.Execute(ctx) }

func (s *authService) LogoutAll(ctx context.Context) (int, error) { return s.logoutAll.Execute(ctx) }

func (s *authService) LogoutSession(ctx context.Context, sessionID string) error {
	return s.logoutSession.Execute(ctx, sessionID)
}

func (s *authService) DeviceChallenge(ctx context.Context, in usecase.DeviceChallengeInput) (*usecase.DeviceChallengeOutput, error) {
	return s.deviceCh.Execute(ctx, in)
}

func (s *authService) DeviceVerify(ctx context.Context, in usecase.DeviceVerifyInput) (*usecase.TokenPair, error) {
	return s.deviceVerify.Execute(ctx, in)
}

func (s *authService) Register(ctx context.Context, in usecase.RegisterInput) (*usecase.RegisterOutput, error) {
	return s.register.Execute(ctx, in)
}

func (s *authService) VerifyEmail(ctx context.Context, token string) error {
	return s.verifyEmail.Execute(ctx, token)
}
