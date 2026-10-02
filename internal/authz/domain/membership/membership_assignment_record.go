package membership

import "time"

// MembershipAssignmentRecord is one current assignment of a person to a
// membership: who, which membership, where, until when, and why.
type MembershipAssignmentRecord struct {
	ID             string
	MembershipID   string
	MembershipName string
	AnchorAxis     string
	IdentityID     string
	Username       string
	NodeID         string
	NodeName       string
	Exact          bool
	ValidUntil     *time.Time
	Reason         string
	AssignedAt     time.Time
	AssignedBy     string
}
