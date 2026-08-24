package row

// GrantScope limits a grant to one place on one scope axis.
type GrantScope struct {
	Axis    string
	Ref     string
	Inherit bool
}
