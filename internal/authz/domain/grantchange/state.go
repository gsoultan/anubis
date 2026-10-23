package grantchange

import (
	"slices"
	"strings"
	"time"
)

// State is everything a bulk change can alter about one grant: its picks,
// its end date, and whether it is still in force.
type State struct {
	Pins    []Pin      `json:"scopes"`
	Until   *time.Time `json:"until,omitempty"`
	Revoked bool       `json:"revoked,omitempty"`
}

// Same reports whether two states would decide the same: the same picks in
// any order, the same end date to the second, the same revoked flag. Undo
// is refused unless the grant is still exactly as the change left it.
func (s State) Same(o State) bool {
	if s.Revoked != o.Revoked {
		return false
	}
	if (s.Until == nil) != (o.Until == nil) {
		return false
	}
	if s.Until != nil && s.Until.Unix() != o.Until.Unix() {
		return false
	}
	return slices.Equal(s.sortedPins(), o.sortedPins())
}

// SamePins compares only the picks, which decides whether a write has to
// touch grant_scopes at all.
func (s State) SamePins(o State) bool {
	return slices.Equal(s.sortedPins(), o.sortedPins())
}

func (s State) sortedPins() []Pin {
	out := slices.Clone(s.Pins)
	slices.SortFunc(out, func(a, b Pin) int {
		if c := strings.Compare(a.Axis, b.Axis); c != 0 {
			return c
		}
		return strings.Compare(a.NodeID, b.NodeID)
	})
	return out
}

// includes lists the places picked in on one kind of place.
func (s State) includes(axis string) []string {
	var out []string
	for _, p := range s.Pins {
		if p.Axis == axis && !p.Exclude {
			out = append(out, p.NodeID)
		}
	}
	return out
}

func (s State) hasExcludes(axis string) bool {
	for _, p := range s.Pins {
		if p.Axis == axis && p.Exclude {
			return true
		}
	}
	return false
}

func (s State) pin(axis, node string) (Pin, bool) {
	for _, p := range s.Pins {
		if p.Axis == axis && p.NodeID == node {
			return p, true
		}
	}
	return Pin{}, false
}

// without drops every pin on axis for which drop says yes.
func (s State) without(axis string, drop func(Pin) bool) State {
	out := s
	out.Pins = make([]Pin, 0, len(s.Pins))
	for _, p := range s.Pins {
		if p.Axis == axis && drop(p) {
			continue
		}
		out.Pins = append(out.Pins, p)
	}
	return out
}

func (s State) with(p Pin) State {
	out := s
	out.Pins = append(slices.Clone(s.Pins), p)
	return out
}
