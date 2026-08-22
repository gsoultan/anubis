package usecase

import "context"

// TokenIssuer mints the PASETO access token and (unless AccessOnly) the
// opaque rotating refresh token for a session.
type TokenIssuer interface {
	Issue(ctx context.Context, in IssueInput) (*TokenPair, error)
}
