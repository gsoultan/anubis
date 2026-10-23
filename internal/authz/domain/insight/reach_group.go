package insight

// ReachGroup is the people holding one role whose picks name every sibling
// of a place but not the place.
type ReachGroup struct {
	RoleID   string
	RoleName string
	People   int
	Risk     string
}
