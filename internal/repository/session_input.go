package repository

import "time"

type SessionInput struct {
	IdentityID    string
	TenantID      string
	ApplicationID string // "" = none
	AMR           []string
	DeviceFP      string
	IP            string
	UserAgent     string
	ActiveScopes  []byte // jsonb map
	ExpiresAt     time.Time
}
