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
