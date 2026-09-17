package grant

type GrantScopeInput struct {
	Axis    string
	NodeID  string
	Inherit bool
	// Exclude carves this place back out of the grant's includes on the same
	// axis. It narrows THIS grant only — another grant covering the same
	// place still allows there (ADR-0004).
	Exclude bool
}
