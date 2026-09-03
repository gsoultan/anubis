package snapshot

import "strings"

// ScopeIndex is the scope hierarchy in the only shape the gate needs: dense
// node indices plus one parent pointer each.
//
// It replaces a materialised closure (Up[descendant][ancestor] = depth). The
// evaluator's sole question is "does granted node A cover target B" — is A an
// ancestor-or-self of B — which a walk up parent[] answers exactly. The
// closure stored that answer precomputed at O(nodes x depth) rows; this
// stores O(nodes) and recomputes it in a handful of integer loads, which at
// real hierarchy depths beats the second map probe it replaces.
//
// Measured through LoadSnapshot on a real 1,010,101-node tenant:
//
//	snapshot resident   530.7 MB -> 91.9 MB
//	live heap objects   1,014,000 -> 4,245
//	load                1613 ms -> ~340 ms
//
// And it no longer grows with depth: 75 B/node at depth 3 and at depth 11,
// where the closure form went from 296 to 527 B/node over the same range.
//
// scope_closure still exists in the database and authorize() still uses it.
// The two are now INDEPENDENT derivations of the same relation, which is what
// makes test/integration/snapshot_parity_test.go worth running.
type ScopeIndex struct {
	idx    map[string]int32 // node id -> dense index
	parent []int32          // parent[i], or -1 at an axis root
}

// ScopeNode is one row of the hierarchy as the loader reads it.
// Parent is empty at an axis root.
type ScopeNode struct {
	ID     string
	Parent string
}

// NewScopeIndex interns node ids and links parents. Rows may arrive in any
// order — ids are assigned first, parents linked in a second pass. A parent
// id that is not itself in the set becomes a root: the walk stops there,
// which denies rather than ascending into nodes we do not have.
//
// The ids are copied into ONE string and the map keys are slices of it. The
// driver hands back a separately allocated 36-byte string per id; keeping a
// million of those alive costs the per-object overhead AND — the part that
// shows up in request latency rather than in RSS — a million more pointers
// for the GC to trace on every scan. Slabbing them took a 1M-node snapshot
// from 1,014,000 live objects to 4,245, and 103.5 MB to 91.9 MB, at the cost
// of one extra pass (~20 ms) on a load that happens on refresh, never on the
// request path.
func NewScopeIndex(nodes []ScopeNode) ScopeIndex {
	total := 0
	for i := range nodes {
		total += len(nodes[i].ID)
	}
	var b strings.Builder
	b.Grow(total)
	for i := range nodes {
		b.WriteString(nodes[i].ID)
	}
	slab := b.String() // Builder hands over its buffer; this does not copy

	x := ScopeIndex{
		idx:    make(map[string]int32, len(nodes)),
		parent: make([]int32, len(nodes)),
	}
	for i, pos := 0, 0; i < len(nodes); i++ {
		n := len(nodes[i].ID)
		x.idx[slab[pos:pos+n]] = int32(i)
		pos += n
	}
	for i := range nodes {
		p, ok := x.idx[nodes[i].Parent]
		if !ok || nodes[i].Parent == "" {
			x.parent[i] = -1
			continue
		}
		x.parent[i] = p
	}
	return x
}

// Len reports the number of indexed nodes.
func (x ScopeIndex) Len() int { return len(x.parent) }

// Resolve maps a node id to its dense index. An id the snapshot has never
// seen fails closed: the caller must treat !ok as "no grant can cover this".
func (x ScopeIndex) Resolve(id string) (int32, bool) {
	n, ok := x.idx[id]
	return n, ok
}

// Intern resolves each constraint's node id to a dense index, in place.
// A node id absent from the index stays unresolved, and an unresolved
// constraint never matches — see ScopeConstraint.node.
func (x ScopeIndex) Intern(cs []ScopeConstraint) {
	for i := range cs {
		if n, ok := x.idx[cs[i].NodeID]; ok {
			cs[i].node = n + 1
		}
	}
}

// CoveredBy reports whether any constraint covers target, mirroring the
// closure probe in migration 0013:  covered && (inherit OR depth = 0).
//
// One walk serves every constraint. The exact (depth 0) case is settled
// before ascending, so a grant with no inheriting constraint never walks.
func (x ScopeIndex) CoveredBy(target int32, cs []ScopeConstraint) bool {
	if target < 0 || int(target) >= len(x.parent) {
		return false
	}
	inherits := false
	for i := range cs {
		if cs[i].node == target+1 {
			return true // depth 0 satisfies with or without inherit
		}
		inherits = inherits || cs[i].Inherit
	}
	if !inherits {
		return false
	}
	// The step bound is a cycle guard, not an optimisation. A simple path to
	// the root visits at most len(parent) nodes, so exceeding that means
	// parent[] contains a loop — and this is the request path, where a
	// non-terminating walk costs a goroutine permanently. Cycles are not
	// reachable through the API (parent_id is written only by
	// scope_move_node, which probes the closure first), but the closure map
	// this replaced could not loop at all, and that property is worth
	// keeping for free. Running out of steps denies.
	for n, steps := x.parent[target], 0; n >= 0 && steps < len(x.parent); n, steps = x.parent[n], steps+1 {
		for i := range cs {
			if cs[i].node == n+1 && cs[i].Inherit {
				return true
			}
		}
	}
	return false
}
