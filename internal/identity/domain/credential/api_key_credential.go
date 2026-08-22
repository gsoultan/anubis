package credential

import "time"

type APIKeyCredential struct {
	ID             string
	IdentityID     string
	TenantID       string
	SecretHash     string
	ExpiresAt      *time.Time
	IdentityStatus string
	TokenEpoch     int
	Blocked        bool
}
