package repository

import "time"

type CredentialInput struct {
	IdentityID string
	TenantID   string
	Kind       string
	Secret     string
	LookupKey  string
	Label      string
	Params     []byte
	ExpiresAt  *time.Time
}
