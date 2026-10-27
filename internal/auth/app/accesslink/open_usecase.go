// Package accesslink turns a share's link into a token that opens that one
// record — the way a recipient with no password gets in, and the only thing
// a link can ever become.
//
// What the token is: an access token for the recipient, ten minutes, no
// session, no refresh, amr ["link"], scopes {"_record": …}. What it is not:
// a sign-in. The decision endpoint refuses it for any subject or record but
// its own, the gate refuses it outright, and it lists no places. A link in
// an inbox is therefore worth exactly the record it was made for, for as
// long as the share lasts — and nothing the moment the share ends.
package accesslink

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
)

// OpenUsecase exchanges a link for a record-bound access token.
type OpenUsecase interface {
	OpenAccessLink(ctx context.Context, in OpenInput) (*authapp.TokenPair, error)
}
