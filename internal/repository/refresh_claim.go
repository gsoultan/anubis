package repository

import "time"

type RefreshClaim struct {
	ID         string
	SessionID  string
	TenantID   string
	FamilyID   string
	Generation int
	ExpiresAt  time.Time
}
