package insight

// RoleUsage is how many people hold one role, and how many of them have it
// from a group.
type RoleUsage struct {
	RoleID    string
	People    int
	ViaGroups int
}
