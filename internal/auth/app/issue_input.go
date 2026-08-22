package authapp

import authdomain "github.com/gsoultan/anubis/internal/auth/domain"

// IssueInput asks the TokenIssuer for tokens against an existing session.
type IssueInput struct {
	Session    *authdomain.SessionView
	TenantSlug string
	ClientID   string // application slug tokens are minted for; "" = self
	// RotateFrom, when set, continues an existing refresh family instead of
	// starting one (the refresh flow).
	RotateFrom *authdomain.RefreshClaim
	// AccessOnly skips refresh minting entirely (scope switch).
	AccessOnly bool
}
