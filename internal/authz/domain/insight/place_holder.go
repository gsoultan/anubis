package insight

// PlaceHolder is one person who reaches a place by a pick: the place itself,
// or a place above it with everything inside. GroupName is set when the
// access comes from a group.
type PlaceHolder struct {
	IdentityID string
	Username   string
	RoleName   string
	GroupName  string
	OnlyHere   bool
	FromAbove  bool
}
