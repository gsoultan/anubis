package signin

import authapp "github.com/gsoultan/anubis/internal/auth/app"

// LoginOutput is tokens, an MFA challenge, or an enrolment refusal — never
// more than one of those three.
//
// Enrolment is the exception to "never both": a member inside the grace
// period gets Tokens AND Enrolment, because the point of a grace period is
// that they can still sign in while being told the date.
type LoginOutput struct {
	Tokens *authapp.TokenPair
	MFA    *authapp.MFAChallenge
	// Enrolment carries a grant token when it stands alone (the deadline has
	// passed and no session is being issued) and none when it accompanies
	// Tokens (still inside the grace period).
	Enrolment *authapp.EnrolmentChallenge
}
