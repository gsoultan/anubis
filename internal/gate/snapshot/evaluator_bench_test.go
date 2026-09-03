package snapshot

import (
	"fmt"
	"testing"
	"time"
)

// BUDGET: the gate serves entirely from this structure with a p99 < 1 ms for
// the whole request, so the decision itself must be sub-microsecond. The
// shape that matters is a BROAD grant (an admin scoped near an axis root)
// evaluated against a deep target — the case ADR-0005 §4 showed is the
// common one, not the edge case.
func BenchmarkEvaluate(b *testing.B) {
	d, identity, perm, targets := benchSnapshot()
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !d.Evaluate(identity, perm, targets, now) {
			b.Fatal("expected allow")
		}
	}
}

// The deny path must not be slower than the allow path: a gate that answers
// denials late is a gate an attacker can distinguish.
func BenchmarkEvaluateDeny(b *testing.B) {
	d, identity, perm, _ := benchSnapshot()
	elsewhere := map[string]string{"org": "node-elsewhere"}
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if d.Evaluate(identity, perm, elsewhere, now) {
			b.Fatal("expected deny")
		}
	}
}

func benchSnapshot() (*Data, string, string, map[string]string) {
	const identity, role, perm = "usr", "role", "billing:invoice:approve"
	d := &Data{
		StrictAxes:      map[string]bool{},
		RolePermissions: map[string]map[string]bool{role: {perm: true}},
		Permissions:     map[string]Permission{perm: {Key: perm, MinAssurance: 1}},
		Identities:      map[string]Identity{identity: {TokenEpoch: 1, AssuranceLevel: 3}},
		RevokedSessions: map[string]bool{},
	}
	// A 6-deep chain under the granted node, plus 500 unrelated nodes so the
	// index is realistically sized.
	nodes := make([]ScopeNode, 0, 512)
	for i := 0; i <= 5; i++ {
		n := ScopeNode{ID: fmt.Sprintf("node-%d", i)}
		if i > 0 {
			n.Parent = fmt.Sprintf("node-%d", i-1)
		}
		nodes = append(nodes, n)
	}
	for i := 0; i < 500; i++ {
		nodes = append(nodes, ScopeNode{ID: fmt.Sprintf("other-%d", i)})
	}
	nodes = append(nodes, ScopeNode{ID: "node-elsewhere"})
	d.Scope = NewScopeIndex(nodes)
	d.GrantsByIdentity = map[string][]Grant{identity: {{
		ID: "g1", RoleID: role, ValidFrom: time.Now().Add(-time.Hour),
		Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "node-0", Inherit: true}}},
	}}}
	d.InternGrantScopes()
	return d, identity, perm, map[string]string{"org": "node-5"}
}
