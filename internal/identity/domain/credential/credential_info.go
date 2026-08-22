package credential

import "time"

type CredentialInfo struct {
	ID         string
	Kind       string
	Label      string
	LookupKey  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}
