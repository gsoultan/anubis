package insight

// ReachEntry is a group whose entry names every sibling of a place but not
// the place: changing the group changes everyone in it.
type ReachEntry struct {
	MembershipID   string
	MembershipName string
	RoleName       string
	Members        int
}
