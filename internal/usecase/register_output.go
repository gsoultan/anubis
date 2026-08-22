package usecase

type RegisterOutput struct {
	IdentityID           string
	VerificationRequired bool
	// VerificationToken exists so the flow is testable end to end before an
	// email sender is wired; production delivery replaces this with mail.
	VerificationToken string
}
