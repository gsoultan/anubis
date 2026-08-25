package controldomain

import "time"

// PlatformAPIKey is a machine credential belonging to an operator. It acts
// AS them, carrying exactly their assignments — so a key can never do more
// than the person who created it, and revoking their access revokes their
// pipeline's at the same moment.
type PlatformAPIKey struct {
	ID             string
	PlatformUserID string
	Username       string
	Label          string
	// Lookup is the public half, safe to display: it identifies the row
	// without being usable on its own.
	Lookup     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	// ExpiresAt is required by the schema. A credential that administers the
	// installation should not outlive the reason it was made.
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// Live reports whether the key may still authenticate.
func (k PlatformAPIKey) Live(now time.Time) bool {
	return k.RevokedAt == nil && k.ExpiresAt.After(now)
}

// PlatformAPIKeyAuth is what authentication needs: the secret to compare,
// and the owner's current standing.
type PlatformAPIKeyAuth struct {
	ID             string
	PlatformUserID string
	Username       string
	SecretHash     string
	ExpiresAt      time.Time
	// OwnerStatus is read on every request rather than trusted from when the
	// key was minted: disabling an operator must stop their automation now.
	OwnerStatus string
	TokenEpoch  int
}
