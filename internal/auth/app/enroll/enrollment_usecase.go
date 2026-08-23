package enroll

import "context"

// EnrollmentUsecase provisions second factors for the CALLING identity.
// Enrolment is two-phase for TOTP: the secret is only persisted once the
// user proves they can generate a code from it, so a bad transfer to the
// authenticator app cannot lock someone out of their own account.
type EnrollmentUsecase interface {
	BeginTOTP(ctx context.Context) (*TOTPEnrollment, error)
	ConfirmTOTP(ctx context.Context, enrollmentToken, code string) (*TOTPConfirmation, error)
	EnrollDeviceKey(ctx context.Context, publicKey, label string) (credentialID string, err error)
}
