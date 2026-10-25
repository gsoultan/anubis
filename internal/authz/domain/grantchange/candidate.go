package grantchange

// Candidate is one live grant of the role a change is about, as it stands.
type Candidate struct {
	GrantID    string
	IdentityID string
	Username   string
	// SelfScoped grants reach only the person's own records; places mean
	// nothing to them.
	SelfScoped bool
	// FromGroup grants are kept in step by their group. Changing one here
	// would be undone by the next resync, so the group is what to change.
	FromGroup bool
	State     State
}
