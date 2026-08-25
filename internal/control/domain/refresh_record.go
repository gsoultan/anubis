package controldomain

import "time"

// PlatformRefresh is one link in an operator session's refresh chain. The
// family is the sign-in itself: the first link's id, carried by every
// successor, so revocation kills the session however many times it rotated.
type PlatformRefresh struct {
	ID             string
	PlatformUserID string
	FamilyID       string
	CreatedAt      time.Time
	// ExpiresAt is absolute for the whole family: rotation never extends a
	// session, it only carries one across token lifetimes.
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

// Spent reports whether presenting this token again is theft: it was
// already consumed by a rotation, or its family was revoked.
func (r PlatformRefresh) Spent() bool {
	return r.UsedAt != nil || r.RevokedAt != nil
}
