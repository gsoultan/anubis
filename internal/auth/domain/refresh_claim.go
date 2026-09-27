package authdomain

import "time"

type RefreshClaim struct {
	ID         string
	SessionID  string
	TenantID   string
	FamilyID   string
	Generation int
	ExpiresAt  time.Time
	// ClientID is the application the family was issued to; "" = none.
	ClientID string
}
