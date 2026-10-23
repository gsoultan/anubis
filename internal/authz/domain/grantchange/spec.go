package grantchange

import "time"

// The kinds of bulk change. Each changes one thing about every grant it
// matches; anything bigger is a new grant, given on a person's page.
const (
	AddPlace    = "add_place"
	RemovePlace = "remove_place"
	SwapPlace   = "swap_place"
	Grow        = "grow"
	SetUntil    = "set_until"
	Revoke      = "revoke"
)

// MaxGrants caps one change. A change is something a person reads in a
// preview and stands behind; past this it is a migration, not a decision.
const MaxGrants = 10000

// Spec is a bulk change as asked: which grants, and what to do to them.
type Spec struct {
	RoleID string `json:"role_id"`
	// Axis with Covers: only grants whose picks on Axis name every one of
	// these places.
	Covers []string `json:"covers,omitempty"`
	// SiblingsOf: only grants whose picks name every other place under this
	// one's parent, and not this one.
	SiblingsOf string `json:"siblings_of,omitempty"`

	Kind     string     `json:"kind"`
	Axis     string     `json:"axis,omitempty"`
	NodeID   string     `json:"node,omitempty"`
	ToNodeID string     `json:"to_node,omitempty"`
	Inherit  bool       `json:"inherit,omitempty"`
	Until    *time.Time `json:"until,omitempty"`
	Reason   string     `json:"reason,omitempty"`
}

// NeedsAxis reports whether the kind acts on one kind of place.
func (s Spec) NeedsAxis() bool {
	switch s.Kind {
	case AddPlace, RemovePlace, SwapPlace, Grow:
		return true
	}
	return false
}
