package authsvc

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	"github.com/gsoultan/anubis/internal/auth/app/clientcreds"
	"github.com/gsoultan/anubis/internal/auth/app/device"
	"github.com/gsoultan/anubis/internal/auth/app/enroll"
	"github.com/gsoultan/anubis/internal/auth/app/mfa"
	sessionapp "github.com/gsoultan/anubis/internal/auth/app/session"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
)

// authService delegates each method to its usecase.
type authService struct {
	login         signin.LoginUsecase
	verifyMfa     mfa.VerifyMfaUsecase
	refresh       tokenapp.RefreshUsecase
	logout        sessionapp.LogoutUsecase
	logoutAll     sessionapp.LogoutAllUsecase
	logoutSession sessionapp.LogoutSessionUsecase
	deviceCh      device.DeviceChallengeUsecase
	deviceVerify  device.DeviceVerifyUsecase
	register      registration.RegisterUsecase
	verifyEmail   registration.VerifyEmailUsecase
	enrollment    enroll.EnrollmentUsecase
	clientCreds   clientcreds.ClientCredentialsUsecase
}

func NewAuthService(
	login signin.LoginUsecase,
	verifyMfa mfa.VerifyMfaUsecase,
	refresh tokenapp.RefreshUsecase,
	logout sessionapp.LogoutUsecase,
	logoutAll sessionapp.LogoutAllUsecase,
	logoutSession sessionapp.LogoutSessionUsecase,
	deviceCh device.DeviceChallengeUsecase,
	deviceVerify device.DeviceVerifyUsecase,
	register registration.RegisterUsecase,
	verifyEmail registration.VerifyEmailUsecase,
	enrollment enroll.EnrollmentUsecase,
	clientCreds clientcreds.ClientCredentialsUsecase,
) AuthService {
	return &authService{
		login: login, verifyMfa: verifyMfa, refresh: refresh,
		logout: logout, logoutAll: logoutAll, logoutSession: logoutSession,
		deviceCh: deviceCh, deviceVerify: deviceVerify,
		register: register, verifyEmail: verifyEmail,
		enrollment: enrollment, clientCreds: clientCreds,
	}
}

func (s *authService) Login(ctx context.Context, in signin.LoginInput) (*signin.LoginOutput, error) {
	return s.login.Execute(ctx, in)
}

func (s *authService) VerifyMfa(ctx context.Context, in mfa.VerifyMfaInput) (*authapp.TokenPair, error) {
	return s.verifyMfa.Execute(ctx, in)
}

func (s *authService) Refresh(ctx context.Context, in tokenapp.RefreshInput) (*authapp.TokenPair, error) {
	return s.refresh.Execute(ctx, in)
}

func (s *authService) Logout(ctx context.Context) error { return s.logout.Execute(ctx) }

func (s *authService) LogoutAll(ctx context.Context) (int, error) { return s.logoutAll.Execute(ctx) }

func (s *authService) LogoutSession(ctx context.Context, sessionID string) error {
	return s.logoutSession.Execute(ctx, sessionID)
}

func (s *authService) DeviceChallenge(ctx context.Context, in device.DeviceChallengeInput) (*device.DeviceChallengeOutput, error) {
	return s.deviceCh.Execute(ctx, in)
}

func (s *authService) DeviceVerify(ctx context.Context, in device.DeviceVerifyInput) (*authapp.TokenPair, error) {
	return s.deviceVerify.Execute(ctx, in)
}

func (s *authService) Register(ctx context.Context, in registration.RegisterInput) (*registration.RegisterOutput, error) {
	return s.register.Execute(ctx, in)
}

func (s *authService) VerifyEmail(ctx context.Context, token string) error {
	return s.verifyEmail.Execute(ctx, token)
}

func (s *authService) BeginTotpEnrollment(ctx context.Context, grantToken string) (*enroll.TOTPEnrollment, error) {
	return s.enrollment.BeginTOTP(ctx, grantToken)
}

func (s *authService) ConfirmTotpEnrollment(ctx context.Context, enrollmentToken, code, grantToken string) (*enroll.TOTPConfirmation, error) {
	return s.enrollment.ConfirmTOTP(ctx, enrollmentToken, code, grantToken)
}

func (s *authService) EnrollDeviceKey(ctx context.Context, publicKey, label string) (string, error) {
	return s.enrollment.EnrollDeviceKey(ctx, publicKey, label)
}

func (s *authService) ClientCredentials(ctx context.Context, in clientcreds.ClientCredentialsInput) (*clientcreds.ClientCredentialsOutput, error) {
	return s.clientCreds.Execute(ctx, in)
}
