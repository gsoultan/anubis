package enroll

import (
	"strings"
	"time"

	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// bindingWindow is how recently a session must have proved its credentials to
// bind a new authenticator to its account. Long enough to scan a QR code
// between BeginTotpEnrollment and ConfirmTotpEnrollment; short enough that a
// token which outlived its sign-in — refreshed, leaked, left in a log —
// cannot plant one.
const bindingWindow = 10 * time.Minute

// mayBindAuthenticator decides whether a SESSION may add an authenticator to
// its own account.
//
// An authenticator is a way in. A device key signs its holder in with no
// password at all and survives a password change. Anubis's own API accepts a
// tenant token whatever application it was minted for, and enrolment used to
// ask for nothing more than a session — so an application a person signed in
// to could take the token it was given, enrol its OWN key on their account,
// and sign in as them from then on. Two rules close that:
//
//   - the token must be one Anubis issued for itself, not one issued to an
//     application (Principal.FirstParty): binding an authenticator is not
//     something an application does on a person's behalf;
//   - the sign-in must be recent, so a token that outlived the moment of
//     sign-in cannot bind one either.
//
// The enrol-grant path is not a session and does not come through here: a
// grant is minted by Anubis's own refused sign-in, redeemed on its own hosted
// page, and only ever adds the FIRST factor.
func mayBindAuthenticator(p *authctx.Principal, now time.Time) error {
	if !p.FirstParty() {
		return apperr.ErrPermissionDenied.
			With("audience", strings.Join(p.Audience, ",")).
			With("hint", "authenticators are enrolled with a token Anubis issued for itself, not one issued to an application")
	}
	if now.Sub(p.AuthTime) > bindingWindow {
		return apperr.ErrStepUpRequired.
			With("max_auth_age", bindingWindow.String()).
			With("hint", "sign in again, then enrol")
	}
	return nil
}
