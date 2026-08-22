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
	Token      string
}
