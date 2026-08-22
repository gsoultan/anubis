package usecase

type VerifyMfaInput struct {
	MFAToken string
	Method   string // "totp"
	Code     string
}
