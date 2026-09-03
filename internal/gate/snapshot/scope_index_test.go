package snapshot

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// chain builds root -> n0 -> n1 -> ... plus a detached sibling subtree.
func chain(depth int) ScopeIndex {
	nodes := []ScopeNode{{ID: "root"}}
	for i := 0; i < depth; i++ {
		parent := "root"
		if i > 0 {
			parent = fmt.Sprintf("n%d", i-1)
		}
		nodes = append(nodes, ScopeNode{ID: fmt.Sprintf("n%d", i), Parent: parent})
	}
	nodes = append(nodes, ScopeNode{ID: "cousin", Parent: "root"})
	return NewScopeIndex(nodes)
}

func mustResolve(t *testing.T, x ScopeIndex, id string) int32 {
	t.Helper()
	n, ok := x.Resolve(id)
	if !ok {
		t.Fatalf("resolve %q: not in index", id)
	}
	return n
}

// interned mirrors what the loader does, so the tests exercise the real path.
func interned(x ScopeIndex, cs ...ScopeConstraint) []ScopeConstraint {
	x.Intern(cs)
	return cs
}

func TestCoversAncestorOrSelf(t *testing.T) {
	x := chain(5)
	cs := interned(x, ScopeConstraint{NodeID: "n0", Inherit: true})
	for _, id := range []string{"n0", "n1", "n2", "n3", "n4"} {
		if !x.CoveredBy(mustResolve(t, x, id), cs) {
			t.Errorf("inherit grant on n0 should cover %s", id)
		}
	}
	for _, id := range []string{"root", "cousin"} {
		if x.CoveredBy(mustResolve(t, x, id), cs) {
			t.Errorf("inherit grant on n0 must NOT cover %s", id)
		}
	}
}

// inherit=false is the closure's depth-0 case: the node itself and nothing under it.
func TestInheritFalseIsSelfOnly(t *testing.T) {
	x := chain(5)
	cs := interned(x, ScopeConstraint{NodeID: "n1", Inherit: false})
	if !x.CoveredBy(mustResolve(t, x, "n1"), cs) {
		t.Error("non-inherit grant must cover its own node")
	}
	for _, id := range []string{"n2", "n3", "n4"} {
		if x.CoveredBy(mustResolve(t, x, id), cs) {
			t.Errorf("non-inherit grant on n1 must NOT cover descendant %s", id)
		}
	}
}

// OR within an axis: any one constraint covering the target is enough.
func TestOrWithinAxis(t *testing.T) {
	x := chain(5)
	cs := interned(x,
		ScopeConstraint{NodeID: "cousin", Inherit: true},
		ScopeConstraint{NodeID: "n2", Inherit: true},
	)
	if !x.CoveredBy(mustResolve(t, x, "n4"), cs) {
		t.Error("second constraint covers n4; grant should be satisfied")
	}
}

func TestUnknownTargetDenies(t *testing.T) {
	x := chain(3)
	if _, ok := x.Resolve("does-not-exist"); ok {
		t.Fatal("resolve should fail for an unknown node")
	}
	cs := interned(x, ScopeConstraint{NodeID: "root", Inherit: true})
	if x.CoveredBy(-1, cs) {
		t.Error("an unresolved target must deny")
	}
	if x.CoveredBy(int32(x.Len()), cs) {
		t.Error("an out-of-range index must deny, not panic")
	}
}

// A parent id outside the loaded set (never expected — the composite FK is on
// tenant_id) must stop the walk rather than ascend into nodes we do not have.
func TestOrphanParentStopsWalk(t *testing.T) {
	x := NewScopeIndex([]ScopeNode{{ID: "child", Parent: "elsewhere"}})
	cs := interned(x, ScopeConstraint{NodeID: "child", Inherit: true})
	if !x.CoveredBy(mustResolve(t, x, "child"), cs) {
		t.Error("child should still cover itself")
	}
	if _, ok := x.Resolve("elsewhere"); ok {
		t.Error("a parent outside the set must not be indexed")
	}
}

// THE fail-open guard. ScopeConstraint.node is offset by one precisely so the
// zero value cannot alias a real index. If someone drops the Intern call, this
// test fails instead of the gate silently granting on whichever node loaded
// first.
func TestUninternedConstraintDenies(t *testing.T) {
	x := chain(3)
	raw := []ScopeConstraint{{NodeID: "root", Inherit: true}} // deliberately NOT interned
	for _, id := range []string{"root", "n0", "n1", "n2"} {
		if x.CoveredBy(mustResolve(t, x, id), raw) {
			t.Fatalf("un-interned constraint matched %s — this is fail-open", id)
		}
	}
}

// Same guard, one level up: a Data whose grants were never interned denies.
func TestEvaluateDeniesWithoutInterning(t *testing.T) {
	const identity, role, perm = "usr", "role", "app:doc:read"
	d := &Data{
		StrictAxes:      map[string]bool{},
		Scope:           chain(3),
		RolePermissions: map[string]map[string]bool{role: {perm: true}},
		Permissions:     map[string]Permission{perm: {Key: perm, MinAssurance: 1}},
		Identities:      map[string]Identity{identity: {TokenEpoch: 1, AssuranceLevel: 3}},
		GrantsByIdentity: map[string][]Grant{identity: {{
			ID: "g1", RoleID: role, ValidFrom: time.Now().Add(-time.Hour),
			Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "root", Inherit: true}}},
		}}},
	}
	targets := map[string]string{"org": "n1"}
	if d.Evaluate(identity, perm, targets, time.Now()) {
		t.Fatal("grants that were never interned must deny")
	}
	d.InternGrantScopes()
	if !d.Evaluate(identity, perm, targets, time.Now()) {
		t.Fatal("after interning the same grant must allow")
	}
}

// Guard rail for the thing this design exists to fix: a regression toward
// per-node maps shows up in the benchmark diff instead of in production RSS.
//
// This is the FULL retained cost per node: the index owns its id strings now
// (NewScopeIndex slabs them), so nothing is hiding in a caller's slice.
//
// What matters is the shape of the curve as much as the number: the closure
// form grew with depth (296 -> 527 B/node from depth 3 to depth 10 at 1M
// nodes) and this must not. Expect the two depths below to agree to within a
// byte.
func BenchmarkScopeIndexMemory(b *testing.B) {
	// branch factors chosen to land on ~depth 3 and ~depth 10 for n nodes.
	for _, branch := range []int{60, 3} {
		const n = 200_000
		nodes := make([]ScopeNode, n)
		for i := range nodes {
			nodes[i] = ScopeNode{ID: fmt.Sprintf("%036d", i)}
			if i > 0 {
				nodes[i].Parent = fmt.Sprintf("%036d", (i-1)/branch)
			}
		}
		x := NewScopeIndex(nodes)
		maxDepth := 0
		for i := int32(0); i < int32(n); i++ {
			d := 0
			for p := x.parent[i]; p >= 0; p = x.parent[p] {
				d++
			}
			if d > maxDepth {
				maxDepth = d
			}
		}
		b.Run(fmt.Sprintf("depth=%d", maxDepth), func(b *testing.B) {
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			built := NewScopeIndex(nodes)
			runtime.GC()
			runtime.ReadMemStats(&after)
			// signed: a re-run of this closure frees the previous index, so
			// the delta can legitimately be negative if measured unsigned.
			alloc := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			b.ReportMetric(float64(alloc)/n, "B/node")
			b.ReportMetric(0, "ns/op")
			runtime.KeepAlive(built)
		})
	}
}

// A cycle in parent[] must not hang the request path. Unreachable through the
// API — parent_id is written only by scope_move_node, which probes the
// closure for a cycle first — but the closure map this replaced could not
// loop at all, and CoveredBy runs per grant per axis on every decision.
// Deadlocking a goroutine on corrupt data is a worse failure than denying.
func TestACycleInParentPointersDoesNotHang(t *testing.T) {
	x := NewScopeIndex([]ScopeNode{
		{ID: "a", Parent: "c"},
		{ID: "b", Parent: "a"},
		{ID: "c", Parent: "b"}, // a -> c -> b -> a
	})
	cs := interned(x, ScopeConstraint{NodeID: "elsewhere", Inherit: true})

	done := make(chan bool, 1)
	go func() { done <- x.CoveredBy(mustResolve(t, x, "a"), cs) }()
	select {
	case got := <-done:
		if got {
			t.Error("a cycle must not manufacture coverage")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CoveredBy did not terminate on a cyclic parent chain")
	}
}

// The bound must not cut a legitimate walk short: a chain as long as the
// index itself still has to resolve.
func TestTheCycleGuardDoesNotTruncateALongChain(t *testing.T) {
	const depth = 500
	nodes := []ScopeNode{{ID: "n0"}}
	for i := 1; i <= depth; i++ {
		nodes = append(nodes, ScopeNode{ID: fmt.Sprintf("n%d", i), Parent: fmt.Sprintf("n%d", i-1)})
	}
	x := NewScopeIndex(nodes)
	cs := interned(x, ScopeConstraint{NodeID: "n0", Inherit: true})
	if !x.CoveredBy(mustResolve(t, x, fmt.Sprintf("n%d", depth)), cs) {
		t.Errorf("a root grant stopped covering a node %d levels down", depth)
	}
}
