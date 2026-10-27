// Package emaillink signs a person in with a link sent to their address —
// the door for a population that has no password, such as applicants.
//
// It is a real sign-in: the realm must allow the "email_link" factor, the
// link is single use and short-lived, and what it opens is an ordinary
// browser session (amr ["email_link"]) that goes on through the OIDC flow
// the person started. Nothing about it is a share or an access link.
package emaillink

import "context"

// Usecase is the two halves of the door.
type Usecase interface {
	// Request answers the same for every username — nil — and mails a link
	// when there is an account to mail. The only error is one that is true
	// for everybody: the server cannot send mail.
	Request(ctx context.Context, in RequestInput) error
	// Complete consumes the link. Invalid, lapsed, used, or for an account
	// that can no longer sign in all answer ErrTokenInvalid.
	Complete(ctx context.Context, token string) (*Completed, error)
}
