package grantchange

// AxisFacts is what planning needs to know about the places involved.
type AxisFacts struct {
	// Deny: on this kind of place, no picks means no access rather than
	// every place, so "everything" has to be written as the top place.
	Deny   bool
	RootID string
	// Siblings of the SiblingsOf place: the other places under its parent.
	Siblings []string
	// Below holds every place inside Spec.NodeID, for growing into a branch.
	Below map[string]bool
}
