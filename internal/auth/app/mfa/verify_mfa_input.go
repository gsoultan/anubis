package mfa

type VerifyMfaInput struct {
	MFAToken string
	Method   string // "totp"
	Code     string
}
