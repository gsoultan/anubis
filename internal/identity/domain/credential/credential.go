package credential

import "time"

type Credential struct {
	ID         string
	IdentityID string
	TenantID   string
	Kind       string
	Secret     string
	// SecretKid names the local key Secret was sealed under. Empty means the
	// enrolment predates the column and the reader must fall back.
	SecretKid   string
	Params      []byte
	SignCounter int64
	// ExpiresAt is set on a credential that was never meant to last: an
	// imported password hash lives until its owner signs in once or this
	// passes, whichever is first. Nil is the ordinary case.
	ExpiresAt *time.Time
}

// Expired reports whether the credential may no longer be used at now.
func (c *Credential) Expired(now time.Time) bool {
	return c.ExpiresAt != nil && !c.ExpiresAt.After(now)
}
