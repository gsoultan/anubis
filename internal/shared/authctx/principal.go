package authctx

import "time"

// Principal is who is calling. Exactly one of the three shapes:
//   - end user:   IdentityID + SessionID set
//   - service:    IdentityID set (service-realm identity), Service=true
//   - anonymous:  zero value, never stored in context
type Principal struct {
	IdentityID string
	TenantID   string
	TenantSlug string
	SessionID  string
	Realm      string
	Roles      []string
	Scopes     map[string]string
	AMR        []string
	AuthTime   time.Time
	IAL        int
	Epoch      int
	Audience   []string
	Service    bool
	// Platform marks a PLATFORM USER: somebody who operates the installation
	// rather than belonging to a tenant (ADR-0011). Such a principal has no
	// tenant and no grants, so authorize() would deny it everything — its
	// authority comes from platform_assignments instead, and only the control
	// context knows how to read it.
	Platform bool
	Token    string
}

// FirstPartyAudience is the audience of a token Anubis issued for itself: a
// sign-in through AuthService.Login, or any sign-in that named no application.
// A token minted for an application carries the application's slug instead.
const FirstPartyAudience = "anubis"

// FirstParty reports whether this token was issued for Anubis itself rather
// than for an application. Managing the account behind a session — enrolling
// an authenticator, listing or ending the user's OTHER sessions — is
// something the account holder does through Anubis, not something an
// application does with a token it was handed. An application acting on such a
// surface with a user's token is how a relying party turns "sign in" into
// standing control of the account.
func (p *Principal) FirstParty() bool {
	for _, a := range p.Audience {
		if a == FirstPartyAudience {
			return true
		}
	}
	return false
}
