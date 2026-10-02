package membership

// MembershipAssignmentFilter selects current assignments: everybody in one
// membership, every membership one person holds, or both. Keyset-paged,
// because a membership can hold thousands of people.
type MembershipAssignmentFilter struct {
	MembershipID string
	IdentityID   string
	Cursor       string
	PageSize     int
}
