package authdomain

import "time"

// APIKeyRecord is one of the tenant's machine credentials.
//
// Deliberately unrelated to a person: the caller it authenticates is the
// tenant's system — a gateway asking authorize(), an integration reading the
// decision API — and its lifetime must not depend on any identity's status.
type APIKeyRecord struct {
	ID       string
	TenantID string
	Label    string
	// Lookup is the indexed public half; the secret is never stored.
	Lookup     string
	CreatedBy  string // platform user's name, for display; empty if gone
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// APIKeyAuth is what the transport needs to authenticate a presented key.
type APIKeyAuth struct {
	ID         string
	TenantID   string
	TenantSlug string
	// TenantStatus lets a suspended tenant's keys stop working with it.
	TenantStatus string
	SecretHash   string
	ExpiresAt    *time.Time
}
