package repository

import "time"

type RefreshInfo struct {
	ID        string
	SessionID string
	TenantID  string
	FamilyID  string
	Status    string
	ExpiresAt time.Time
}
