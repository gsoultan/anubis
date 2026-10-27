package insight

// AppUsage is how many people can use one application today: everyone
// holding a live role in it, given directly or through a group.
type AppUsage struct {
	People    int
	Direct    int
	ViaGroups int
}
