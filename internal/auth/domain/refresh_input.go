package authdomain

import "time"

type RefreshInput struct {
	SessionID  string
	TenantID   string
	FamilyID   string
	Generation int
	TokenHash  []byte
	ExpiresAt  time.Time
	BoundKey   string
}
