package usecase

// LoginOutput is either tokens or an MFA challenge, never both.
type LoginOutput struct {
	Tokens *TokenPair
	MFA    *MFAChallenge
}
