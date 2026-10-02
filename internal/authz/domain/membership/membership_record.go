package membership

type MembershipRecord struct {
	ID          string
	Name        string
	Description string
	// AnchorAxis is the structure a membership applies in WHERE EACH MEMBER IS
	// ASSIGNED; empty when it gives every member the same places. Fixed at
	// creation.
	AnchorAxis string
	// MemberCount is people with a current assignment, not assignments: one
	// person may hold a where-assigned membership at several places.
	MemberCount int
}
