package authzdomain

// EffectiveGrant is one live grant an identity holds, with its scopes.
//
// "Effective" means revocation and validity windows have already been applied:
// everything here is in force right now. It is a point-in-time answer, which is
// what any caller caching it has to assume anyway.
type EffectiveGrant struct {
	ID     string
	RoleID string
	Role   string

	// SelfScoped limits the grant to the identity's own records (`_owner`).
	// A caller that ignores it treats the grant as covering everybody's.
	SelfScoped bool

	// Scopes is empty for an UNSCOPED grant, which in Anubis means every node
	// on every axis — not "no access". A caller that reads empty as none has
	// inverted the meaning.
	Scopes []GrantScope
}

// GrantScope is one node a grant names, and how.
type GrantScope struct {
	Axis   string
	NodeID string

	// Inherit means the grant reaches descendants, not only this node.
	Inherit bool

	// Exclude carves this node OUT of the grant's includes on the same axis.
	// Ignoring it produces a grant WIDER than the one held.
	Exclude bool
}

// HasExclusions reports whether any scope is a carve-out.
func (g EffectiveGrant) HasExclusions() bool {
	for _, s := range g.Scopes {
		if s.Exclude {
			return true
		}
	}
	return false
}
