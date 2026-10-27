// Package share is one person opening ONE record to another for a while.
//
// A share is the narrowest way Anubis says yes: it names a record, not a
// place, and it always ends. authorize() consults it only when the caller
// passes the reserved "_record" target, so an application that never shares
// never pays for it.
package share

import "time"

// Share is a record_shares row.
type Share struct {
	ID             string
	TenantID       string
	FromIdentityID string
	ToIdentityID   string
	// Record is "kind/ref" — see RecordKey.
	Record string
	// RecordScopes is where the record lived when it was shared: the places
	// the sharer was authorised against, kept so a later revoke or listing
	// can be authorised against the same ones.
	RecordScopes map[string]string
	Permissions  []string
	Note         string
	CreatedAt    time.Time
	ValidUntil   time.Time
	RevokedAt    *time.Time
	RevokeReason string
}

// Live reports whether the share still applies at now.
func (s *Share) Live(now time.Time) bool {
	return s.RevokedAt == nil && s.ValidUntil.After(now)
}

// Confers reports whether the share gives this permission.
func (s *Share) Confers(permission string) bool {
	for _, p := range s.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
