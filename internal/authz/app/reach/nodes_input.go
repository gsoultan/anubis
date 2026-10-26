package authzreach

// NodesInput asks where Subject may exercise Permission along Axis.
// Scopes fixes the OTHER axes — the ones the screen already knows, usually
// from the session's active scope — because a grant limited on two axes
// reaches a place on one only where the other is satisfied too. An axis a
// grant limits that is neither Axis nor fixed here drops that grant, the
// same fail-closed rule Authorize applies.
type NodesInput struct {
	Subject    string
	Permission string
	Axis       string
	Scopes     map[string]string
}
