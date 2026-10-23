package insight

// PlaceReach is, for one place, who already reaches it and who holds a list
// naming all of its siblings and so might expect it too.
type PlaceReach struct {
	HaveAll      int
	HaveBranch   int
	MightExpect  []ReachGroup
	GroupEntries []ReachEntry
	OtherFixed   int
}
