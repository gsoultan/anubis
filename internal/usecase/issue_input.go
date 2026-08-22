package usecase

import "github.com/gsoultan/anubis/internal/repository"

// IssueInput asks the TokenIssuer for tokens against an existing session.
type IssueInput struct {
	Session    *repository.SessionView
	TenantSlug string
	ClientID   string // application slug tokens are minted for; "" = self
	// RotateFrom, when set, continues an existing refresh family instead of
	// starting one (the refresh flow).
	RotateFrom *repository.RefreshClaim
	// AccessOnly skips refresh minting entirely (scope switch).
	AccessOnly bool
}
