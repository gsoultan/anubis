package row

// GrantScope limits a grant to one place on one scope axis.
type GrantScope struct {
	Axis    string
	Ref     string
	Inherit bool
	// Exclude carves this place back out of the grant's includes on the same
	// axis. A sheet that names only exclusions on an axis is refused by the
	// database (migration 0046), not quietly widened here.
	Exclude bool
}
