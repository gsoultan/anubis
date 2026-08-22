package usecase

// MFAChallenge is the 202 shape of a login that needs a second factor.
type MFAChallenge struct {
	MFAToken  string
	Methods   []string
	ExpiresIn int
}
