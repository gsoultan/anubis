package signin

import authapp "github.com/gsoultan/anubis/internal/auth/app"

// LoginOutput is either tokens or an MFA challenge, never both.
type LoginOutput struct {
	Tokens *authapp.TokenPair
	MFA    *authapp.MFAChallenge
}
