package insight

// FullList is grants of one role whose picks name every place under one
// parent, one by one: a fixed list that reads as "all" today and will miss
// the next place added there.
type FullList struct {
	Axis       string
	ParentID   string
	ParentName string
	IsRoot     bool
	RoleID     string
	RoleName   string
	Grants     int
	Places     int
}
