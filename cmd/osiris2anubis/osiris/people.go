package osiris

// User is one iam.users row. EntityType is APPLICANT, EMPLOYEE, REGISTRANT
// or DEVICE; Username is the e-mail address Osiris signs people in with.
type User struct {
	ID         string
	Username   string
	EntityType string
	EntityID   string
}

// Placement is one iam.scopes row for a user: where they sit. Any field may
// be empty. Grade and job type are a position, not a place, and are counted
// rather than carried.
type Placement struct {
	ID             string
	UserID         string
	OrganizationID string
	UnitID         string
	WorkplaceID    string
	GradeID        string
	JobTypeID      string
	GroupIDs       []string // iam.members rows for this placement
}

// People is everything about who exists and where they sit.
type People struct {
	Users      []User
	Placements []Placement
}
