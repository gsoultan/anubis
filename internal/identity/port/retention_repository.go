package identityport

import "context"

// RetentionRepository enforces statutory limits. Anonymise, never delete:
// grants and audit entries reference identities, and authorize() already
// denies an anonymised subject (migrations/0009 gate 1).
type RetentionRepository interface {
	// ApplyRealmRetention stamps retention_until on identities that lack one.
	ApplyRealmRetention(ctx context.Context) (int64, error)
	// ExpireRetained anonymises everything past its deadline, returning the
	// PII keys that must now be shredded.
	ExpireRetained(ctx context.Context) (identityIDs []string, tenantIDs []string, piiKeys []string, err error)
	// Anonymize executes a right-to-erasure request immediately.
	Anonymize(ctx context.Context, tenantID, identityID string) (piiKeyID string, err error)
}
