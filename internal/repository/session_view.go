package repository

import "time"

// SessionView joins the session with the identity fields token minting needs.
type SessionView struct {
	ID             string
	IdentityID     string
	TenantID       string
	ApplicationID  string
	AMR            []string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	AuthTime       time.Time
	ExpiresAt      time.Time
	ActiveScopes   []byte
	DeviceFP       string
	TokenEpoch     int
	IdentityStatus string
	AssuranceLevel int
	Username       string
	Email          string
	RealmID        string
	RealmCode      string
}
