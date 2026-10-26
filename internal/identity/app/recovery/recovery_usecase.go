// Package identityrecovery is how a person gets back into, or re-keys, their
// own account: a reset link by email, and a password change while signed in.
//
// Both end by revoking every session and bumping the token epoch. A password
// is changed because the old one is suspected, forgotten, or inherited from
// another system; in none of those cases should a session opened with it
// outlive the change.
package identityrecovery

import "context"

// RecoveryUsecase is the self-service credential surface for tenant people.
// Platform operators have their own (ChangePlatformPassword); this one
// refuses them.
type RecoveryUsecase interface {
	// RequestPasswordReset answers the same way whether or not the account
	// exists, is enabled, or has an email address: nil. The only error it
	// returns is one that is true for every account at once — the server
	// cannot send mail.
	RequestPasswordReset(ctx context.Context, in RequestResetInput) error
	// CompletePasswordReset consumes the emailed token and sets the password.
	// The token is single use: a policy refusal burns it, and the person
	// asks for a new link.
	CompletePasswordReset(ctx context.Context, token, password string) error
	// ChangePassword needs the current password, not merely a session —
	// a token left in a browser must not be enough to re-key the account.
	ChangePassword(ctx context.Context, current, next string) error
}
