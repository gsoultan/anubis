package grantchange

import (
	"errors"
	"slices"
)

// Why a matched grant is left alone. Shown in the preview, so the numbers
// add up and nobody has to guess where the rest went.
const (
	SkipFromGroup    = "comes from a group; change the group instead"
	SkipSelf         = "limited to the person's own records"
	SkipHasAll       = "already has every place of this kind"
	SkipHasPlace     = "already has this place"
	SkipNoPlace      = "does not have this place picked"
	SkipLastPlace    = "it is their only place here; removing it would give every place"
	SkipAlreadyUntil = "already ends then"
	SkipCovered      = "already reaches it"
)

// ErrTooMany refuses a change that would match more than MaxGrants.
var ErrTooMany = errors.New("this change matches more than 10,000 grants; narrow it first")

// Plan is what a change would do: the grants it alters, and how many it
// matched but leaves alone, by reason.
type Plan struct {
	Matched int
	Items   []Item
	Skipped map[string]int
}

// People counts the distinct people the change alters.
func (p Plan) People() int {
	seen := map[string]bool{}
	for _, it := range p.Items {
		seen[it.IdentityID] = true
	}
	return len(seen)
}

// Build works out a change from the grants as they stand. It is pure: the
// same candidates and facts always give the same plan, so the preview a
// person reads is the change that is applied.
func Build(s Spec, cands []Candidate, f AxisFacts) (Plan, error) {
	if len(cands) > MaxGrants {
		return Plan{}, ErrTooMany
	}
	out := Plan{Skipped: map[string]int{}}
	for _, c := range cands {
		if !matches(s, c, f) {
			continue
		}
		out.Matched++
		if c.FromGroup {
			out.Skipped[SkipFromGroup]++
			continue
		}
		if c.SelfScoped && s.NeedsAxis() {
			out.Skipped[SkipSelf]++
			continue
		}
		after, skip := apply(s, c.State, f)
		if skip != "" {
			out.Skipped[skip]++
			continue
		}
		out.Items = append(out.Items, Item{
			GrantID: c.GrantID, IdentityID: c.IdentityID, Username: c.Username,
			Before: c.State, After: after,
		})
	}
	return out, nil
}

func matches(s Spec, c Candidate, f AxisFacts) bool {
	if len(s.Covers) > 0 {
		have := c.State.includes(s.Axis)
		for _, n := range s.Covers {
			if !slices.Contains(have, n) {
				return false
			}
		}
	}
	if s.SiblingsOf != "" {
		if len(f.Siblings) == 0 {
			return false
		}
		have := c.State.includes(s.Axis)
		if slices.Contains(have, s.SiblingsOf) {
			return false
		}
		for _, n := range f.Siblings {
			if !slices.Contains(have, n) {
				return false
			}
		}
	}
	return true
}

// apply returns the state after the change, or why the grant is skipped.
func apply(s Spec, st State, f AxisFacts) (State, string) {
	ax := s.Axis
	inc := st.includes(ax)
	switch s.Kind {
	case AddPlace:
		if len(inc) == 0 && !f.Deny {
			return st, SkipHasAll
		}
		if p, ok := st.pin(ax, s.NodeID); ok && !p.Exclude {
			return st, SkipHasPlace
		}
		// A place left out by name and now added is no longer left out.
		next := st.without(ax, func(p Pin) bool { return p.NodeID == s.NodeID })
		return next.with(Pin{Axis: ax, NodeID: s.NodeID, Inherit: s.Inherit}), ""
	case RemovePlace:
		if !slices.Contains(inc, s.NodeID) {
			return st, SkipNoPlace
		}
		if len(inc) == 1 {
			return st, SkipLastPlace
		}
		return st.without(ax, func(p Pin) bool { return p.NodeID == s.NodeID && !p.Exclude }), ""
	case SwapPlace:
		if !slices.Contains(inc, s.NodeID) {
			return st, SkipNoPlace
		}
		next := st.without(ax, func(p Pin) bool {
			return (p.NodeID == s.NodeID && !p.Exclude) || p.NodeID == s.ToNodeID
		})
		return next.with(Pin{Axis: ax, NodeID: s.ToNodeID, Inherit: s.Inherit}), ""
	case Grow:
		return grow(s, st, f)
	case SetUntil:
		if (st.Until == nil && s.Until == nil) || (st.Until != nil && s.Until != nil && st.Until.Unix() == s.Until.Unix()) {
			return st, SkipAlreadyUntil
		}
		next := st
		next.Until = s.Until
		return next, ""
	case Revoke:
		next := st
		next.Revoked = true
		return next, ""
	}
	return st, "unknown change"
}

func grow(s Spec, st State, f AxisFacts) (State, string) {
	ax := s.Axis
	inc := st.includes(ax)
	if s.NodeID == "" {
		// Everything. Where no picks already means everything, and nothing
		// is left out by name, that is what to write: no picks at all. A
		// deny axis, or a list with exceptions, needs the top place instead,
		// because the exclusion guard requires an include beside an exclude.
		root, rooted := st.pin(ax, f.RootID)
		if f.Deny || st.hasExcludes(ax) {
			if rooted && root.Inherit && !root.Exclude && len(inc) == 1 {
				return st, SkipHasAll
			}
			next := st.without(ax, func(p Pin) bool { return !p.Exclude })
			return next.with(Pin{Axis: ax, NodeID: f.RootID, Inherit: true}), ""
		}
		if len(inc) == 0 {
			return st, SkipHasAll
		}
		return st.without(ax, func(Pin) bool { return true }), ""
	}
	if len(inc) == 0 && !f.Deny {
		return st, SkipHasAll
	}
	if p, ok := st.pin(ax, s.NodeID); ok && p.Inherit && !p.Exclude {
		return st, SkipCovered
	}
	// The branch replaces any pick inside it: they say less than it does.
	next := st.without(ax, func(p Pin) bool {
		return !p.Exclude && (p.NodeID == s.NodeID || f.Below[p.NodeID])
	})
	return next.with(Pin{Axis: ax, NodeID: s.NodeID, Inherit: true}), ""
}
