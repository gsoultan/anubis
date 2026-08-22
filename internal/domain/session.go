package domain

import "time"

// Session is a signed-in device.
type Session struct {
	ID           string
	IdentityID   string
	TenantID     string
	AMR          []string
	AuthTime     time.Time
	ExpiresAt    time.Time
	ActiveScopes map[string]string
}
