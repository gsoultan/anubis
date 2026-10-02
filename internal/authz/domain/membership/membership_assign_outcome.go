package membership

// MembershipAssignOutcome is what an assignment did. AssignmentID is empty
// when the person already held the membership at that place: repeating an
// assignment creates nothing.
type MembershipAssignOutcome struct {
	AssignmentID  string
	GrantsCreated int
}
