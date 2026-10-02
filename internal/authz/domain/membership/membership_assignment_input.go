package membership

import "time"

// MembershipAssignmentInput puts one person in a membership, at one place when
// the membership applies where each member is assigned.
type MembershipAssignmentInput struct {
	MembershipID string
	IdentityID   string
	// NodeID is the place, on the membership's anchor axis. Empty for a
	// membership that gives everyone the same places — and required for one
	// that does not.
	NodeID string
	// Exact limits the place to itself; the default is the place and
	// everything inside it.
	Exact bool
	// ValidUntil ends the assignment, and every grant it materialises, at
	// that moment. Nil runs until somebody removes it.
	ValidUntil *time.Time
	// Reason is kept on the assignment and on each grant it gives.
	Reason string
}
