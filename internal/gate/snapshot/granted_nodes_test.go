package snapshot

import (
	"reflect"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/authz/domain/granted"
)

// A small installation: one organisation axis with a tree, one unit axis.
//
//	org:  impack ─┬─ finance ─── surabaya
//	              └─ hr ─────── jkt
//	unit: u-root ─── u-fin
const (
	subject = "dewi"
	perm    = "training:request:approve"
	role    = "approver"
	now     = 1_700_000_000
)

func grantedProbe() *Data {
	d := &Data{
		StrictAxes:      map[string]bool{},
		RolePermissions: map[string]map[string]bool{role: {perm: true}, "other": {"x:y:z": true}},
		Permissions:     map[string]Permission{perm: {Key: perm, MinAssurance: 1}, "x:y:z": {Key: "x:y:z"}},
		Identities: map[string]Identity{
			subject:   {TokenEpoch: 1, AssuranceLevel: 2},
			"blocked": {TokenEpoch: 1, AssuranceLevel: 2, Blocked: true},
			"nobody":  {TokenEpoch: 1, AssuranceLevel: 2},
		},
		RevokedSessions:  map[string]bool{},
		GrantsByIdentity: map[string][]Grant{},
	}
	d.Scope = NewScopeIndex([]ScopeNode{
		{ID: "impack"}, {ID: "finance", Parent: "impack"}, {ID: "surabaya", Parent: "finance"},
		{ID: "hr", Parent: "impack"}, {ID: "jkt", Parent: "hr"},
		{ID: "u-root"}, {ID: "u-fin", Parent: "u-root"},
	})
	return d
}

func at(sec int64) time.Time { return time.Unix(sec, 0) }

func withGrants(d *Data, gs ...Grant) *Data {
	d.GrantsByIdentity[subject] = append(d.GrantsByIdentity[subject], gs...)
	d.InternGrantScopes()
	return d
}

func TestPlacesReachedWithTheirReachAndExceptions(t *testing.T) {
	d := withGrants(grantedProbe(),
		// finance and everything inside, except surabaya
		Grant{ID: "g1", RoleID: role, Scopes: map[string][]ScopeConstraint{
			"org": {{NodeID: "finance", Inherit: true}, {NodeID: "surabaya", Inherit: true, Exclude: true}},
		}},
		// jkt, this place only
		Grant{ID: "g2", RoleID: role, Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "jkt"}}}},
		// the same answer from a second grant must not appear twice
		Grant{ID: "g3", RoleID: role, Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "jkt"}}}},
		// a role that does not confer the permission contributes nothing
		Grant{ID: "g4", RoleID: "other", Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "impack", Inherit: true}}}},
		// expired
		Grant{ID: "g5", RoleID: role, ValidUntil: at(now - 1), Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "impack", Inherit: true}}}},
	)

	out, known := d.GrantedNodes(subject, perm, "org", nil, at(now))
	if !known {
		t.Fatal("a known subject and permission reported unknown")
	}
	want := []granted.Node{
		{NodeID: "finance", Inherit: true, Except: []granted.Node{{NodeID: "surabaya", Inherit: true}}},
		{NodeID: "jkt"},
	}
	if out.Everywhere || out.OwnRecords || !reflect.DeepEqual(out.Nodes, want) {
		t.Fatalf("got %+v, want nodes %+v", out, want)
	}
}

// The list and the verdict come from the same data and must agree: for every
// place on the axis, Evaluate allows it exactly when the list covers it.
func TestTheListAgreesWithEvaluateOnEveryPlace(t *testing.T) {
	d := withGrants(grantedProbe(),
		Grant{ID: "g1", RoleID: role, Scopes: map[string][]ScopeConstraint{
			"org": {{NodeID: "finance", Inherit: true}, {NodeID: "surabaya", Inherit: true, Exclude: true}},
		}},
		Grant{ID: "g2", RoleID: role, Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "jkt"}}}},
	)
	out, _ := d.GrantedNodes(subject, perm, "org", nil, at(now))

	// Re-express the answer as constraints and ask the same index that
	// Evaluate uses whether each place is covered.
	covered := func(place string) bool {
		target, _ := d.Scope.Resolve(place)
		for _, n := range out.Nodes {
			cs := []ScopeConstraint{{NodeID: n.NodeID, Inherit: n.Inherit}}
			for _, e := range n.Except {
				cs = append(cs, ScopeConstraint{NodeID: e.NodeID, Inherit: e.Inherit, Exclude: true})
			}
			d.Scope.Intern(cs)
			if d.Scope.CoveredBy(target, cs) {
				return true
			}
		}
		return false
	}
	for _, place := range []string{"impack", "finance", "surabaya", "hr", "jkt"} {
		verdict := d.Evaluate(subject, perm, map[string]string{"org": place}, at(now))
		if verdict != covered(place) {
			t.Errorf("%s: Evaluate=%v but the list says %v", place, verdict, covered(place))
		}
	}
}

func TestEverywhereHasNoListAndOwnRecordsIsSeparate(t *testing.T) {
	d := withGrants(grantedProbe(),
		Grant{ID: "g1", RoleID: role, Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "jkt"}}}},
		Grant{ID: "g2", RoleID: role}, // no limit on any axis
		Grant{ID: "g3", RoleID: role, SelfScoped: true},
	)
	out, _ := d.GrantedNodes(subject, perm, "org", nil, at(now))
	if !out.Everywhere || !out.OwnRecords {
		t.Fatalf("got %+v", out)
	}
	if len(out.Nodes) != 0 {
		t.Fatalf("a list beside everywhere invites a narrower filter: %+v", out.Nodes)
	}
}

// A grant limited on a second axis reaches places on the open axis only
// where the second axis is satisfied — fail-closed when it is not fixed.
func TestAGrantLimitedOnAnotherAxisNeedsThatAxisFixed(t *testing.T) {
	d := withGrants(grantedProbe(),
		Grant{ID: "g1", RoleID: role, Scopes: map[string][]ScopeConstraint{
			"org":  {{NodeID: "finance", Inherit: true}},
			"unit": {{NodeID: "u-fin"}},
		}},
	)
	if out, _ := d.GrantedNodes(subject, perm, "org", nil, at(now)); len(out.Nodes) != 0 || out.Everywhere {
		t.Fatalf("unit not fixed, yet the grant reached: %+v", out)
	}
	if out, _ := d.GrantedNodes(subject, perm, "org", map[string]string{"unit": "u-root"}, at(now)); len(out.Nodes) != 0 {
		t.Fatalf("unit fixed outside the grant, yet it reached: %+v", out)
	}
	out, _ := d.GrantedNodes(subject, perm, "org", map[string]string{"unit": "u-fin"}, at(now))
	if len(out.Nodes) != 1 || out.Nodes[0].NodeID != "finance" {
		t.Fatalf("unit fixed inside the grant: %+v", out)
	}
}

// A strict axis a grant does not address kills the grant, exactly as it does
// for Evaluate — so "everywhere" cannot come from a grant the gate would
// refuse.
func TestAStrictAxisDropsTheUnlimitedGrant(t *testing.T) {
	d := withGrants(grantedProbe(), Grant{ID: "g1", RoleID: role})
	d.StrictAxes["org"] = true
	out, _ := d.GrantedNodes(subject, perm, "org", nil, at(now))
	if out.Everywhere || len(out.Nodes) != 0 {
		t.Fatalf("a strict axis was ignored: %+v", out)
	}
}

func TestUnknownIsNotNowhere(t *testing.T) {
	d := withGrants(grantedProbe(), Grant{ID: "g1", RoleID: role})
	if _, known := d.GrantedNodes("stranger", perm, "org", nil, at(now)); known {
		t.Fatal("an unknown subject reported as known")
	}
	if _, known := d.GrantedNodes(subject, "no:such:permission", "org", nil, at(now)); known {
		t.Fatal("an unknown permission reported as known")
	}
	out, known := d.GrantedNodes("blocked", perm, "org", nil, at(now))
	if !known || out.Everywhere || out.OwnRecords || len(out.Nodes) != 0 {
		t.Fatalf("a blocked identity should be known and reach nowhere: %+v known=%v", out, known)
	}
	out, _ = d.GrantedNodes("nobody", perm, "org", nil, at(now))
	if out.Everywhere || out.OwnRecords || len(out.Nodes) != 0 {
		t.Fatalf("no grants should reach nowhere: %+v", out)
	}
}

// Reaches is the entitlement behind SwitchScope: any live grant, any role,
// that puts the subject at the place — by name, from above, or unlimited.
func TestReachesFollowsGrantsNotPermissions(t *testing.T) {
	d := withGrants(grantedProbe(),
		Grant{ID: "g1", RoleID: "other", Scopes: map[string][]ScopeConstraint{
			"org": {{NodeID: "finance", Inherit: true}, {NodeID: "surabaya", Exclude: true}},
		}},
		Grant{ID: "g2", RoleID: role, SelfScoped: true},
	)
	for place, want := range map[string]bool{"finance": true, "surabaya": false, "hr": false, "impack": false, "ghost": false} {
		if got := d.Reaches(subject, "org", place, at(now)); got != want {
			t.Errorf("%s: reaches=%v want %v", place, got, want)
		}
	}
	if d.Reaches("nobody", "org", "finance", at(now)) {
		t.Fatal("no grants, yet reaches")
	}
	// Unlimited on the axis reaches every place on it — unless the axis is
	// strict, in which case the grant is dead.
	e := withGrants(grantedProbe(), Grant{ID: "g1", RoleID: role})
	if !e.Reaches(subject, "org", "jkt", at(now)) {
		t.Fatal("an unlimited grant does not reach")
	}
	e.StrictAxes["org"] = true
	if e.Reaches(subject, "org", "jkt", at(now)) {
		t.Fatal("a strict axis was ignored by Reaches")
	}
}
