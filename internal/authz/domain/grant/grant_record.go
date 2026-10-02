package grant

import "time"

type GrantRecord struct {
	ID              string
	IdentityID      string
	RoleID          string
	RoleName        string
	SelfScoped      bool
	ValidFrom       time.Time
	ValidUntil      *time.Time
	RevokedAt       *time.Time
	GrantedBy       string
	ViaMembershipID string
	// ViaAssignmentID is the membership assignment that gave this grant —
	// one person can hold a membership at several places.
	ViaAssignmentID string
	// Reason is why the access was given; a revoke never writes it.
	Reason string
	// RevokeReason is why it was taken away — empty while live, or when the
	// revoke gave none.
	RevokeReason string
}
