package grantchange

import (
	"testing"
	"time"
)

func cand(id string, pins ...Pin) Candidate {
	return Candidate{GrantID: id, IdentityID: "p-" + id, State: State{Pins: pins}}
}

func inc(node string) Pin { return Pin{Axis: "org", NodeID: node, Inherit: true} }
func exc(node string) Pin { return Pin{Axis: "org", NodeID: node, Exclude: true} }

func TestAddPlaceSkipsThoseWhoHaveEverything(t *testing.T) {
	p, err := Build(Spec{Kind: AddPlace, Axis: "org", NodeID: "c"},
		[]Candidate{cand("1", inc("a"), inc("b")), cand("2"), cand("3", inc("c"))}, AxisFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Matched != 3 || len(p.Items) != 1 || p.Skipped[SkipHasAll] != 1 || p.Skipped[SkipHasPlace] != 1 {
		t.Fatalf("plan = %+v", p)
	}
	if got := p.Items[0].After.includes("org"); len(got) != 3 {
		t.Fatalf("after = %v", got)
	}
}

func TestAddPlaceOnDenyAxisWritesThePlace(t *testing.T) {
	p, _ := Build(Spec{Kind: AddPlace, Axis: "org", NodeID: "c"}, []Candidate{cand("1")}, AxisFacts{Deny: true})
	if len(p.Items) != 1 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestAddPlaceReplacesAnExclusion(t *testing.T) {
	p, _ := Build(Spec{Kind: AddPlace, Axis: "org", NodeID: "c"},
		[]Candidate{cand("1", inc("root"), exc("c"))}, AxisFacts{})
	after := p.Items[0].After
	if pin, _ := after.pin("org", "c"); pin.Exclude || len(after.Pins) != 2 {
		t.Fatalf("after = %+v", after)
	}
}

func TestRemovePlaceNeverLeavesNoPicks(t *testing.T) {
	p, _ := Build(Spec{Kind: RemovePlace, Axis: "org", NodeID: "a"},
		[]Candidate{cand("1", inc("a")), cand("2", inc("a"), inc("b")), cand("3", inc("b"))}, AxisFacts{})
	if len(p.Items) != 1 || p.Skipped[SkipLastPlace] != 1 || p.Skipped[SkipNoPlace] != 1 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestGrowToEverything(t *testing.T) {
	f := AxisFacts{RootID: "root"}
	p, _ := Build(Spec{Kind: Grow, Axis: "org"},
		[]Candidate{cand("1", inc("a"), inc("b")), cand("2"), cand("3", inc("a"), exc("x"))}, f)
	if len(p.Items) != 2 || p.Skipped[SkipHasAll] != 1 {
		t.Fatalf("plan = %+v", p)
	}
	if n := len(p.Items[0].After.Pins); n != 0 {
		t.Fatalf("plain list should become no picks, got %d", n)
	}
	// With an exception, the top place is written so the exception still
	// has an include beside it.
	after := p.Items[1].After
	if r, ok := after.pin("org", "root"); !ok || !r.Inherit || !after.hasExcludes("org") {
		t.Fatalf("after = %+v", after)
	}
}

func TestGrowToEverythingOnDenyAxisWritesTheTop(t *testing.T) {
	p, _ := Build(Spec{Kind: Grow, Axis: "org"},
		[]Candidate{cand("1", inc("a")), cand("2", inc("root"))}, AxisFacts{Deny: true, RootID: "root"})
	if len(p.Items) != 1 || p.Skipped[SkipHasAll] != 1 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestGrowIntoBranchFoldsPicksInside(t *testing.T) {
	f := AxisFacts{Below: map[string]bool{"a1": true, "a2": true}}
	p, _ := Build(Spec{Kind: Grow, Axis: "org", NodeID: "a"},
		[]Candidate{cand("1", inc("a1"), inc("a2"), inc("b"))}, f)
	got := p.Items[0].After.includes("org")
	if len(got) != 2 {
		t.Fatalf("after = %v", got)
	}
}

func TestSiblingsOfFindsFullLists(t *testing.T) {
	f := AxisFacts{Siblings: []string{"a", "b"}}
	s := Spec{Kind: AddPlace, Axis: "org", NodeID: "c", SiblingsOf: "c", RoleID: "r"}
	p, _ := Build(s, []Candidate{cand("1", inc("a"), inc("b")), cand("2", inc("a")), cand("3", inc("a"), inc("b"), inc("c"))}, f)
	if p.Matched != 1 || len(p.Items) != 1 || p.Items[0].GrantID != "1" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestGroupAndSelfGrantsAreLeftAlone(t *testing.T) {
	g := cand("1", inc("a"), inc("b"))
	g.FromGroup = true
	s := cand("2", inc("a"), inc("b"))
	s.SelfScoped = true
	p, _ := Build(Spec{Kind: AddPlace, Axis: "org", NodeID: "c"}, []Candidate{g, s}, AxisFacts{})
	if len(p.Items) != 0 || p.Skipped[SkipFromGroup] != 1 || p.Skipped[SkipSelf] != 1 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestSetUntilAndSame(t *testing.T) {
	at := time.Unix(2000000000, 0)
	c := cand("1", inc("a"))
	p, _ := Build(Spec{Kind: SetUntil, Until: &at}, []Candidate{c}, AxisFacts{})
	if len(p.Items) != 1 || p.Items[0].After.Until == nil {
		t.Fatalf("plan = %+v", p)
	}
	if p.Items[0].Before.Same(p.Items[0].After) {
		t.Fatal("different end dates read as the same")
	}
	again := State{Pins: []Pin{inc("a")}, Until: &at}
	if !p.Items[0].After.Same(again) {
		t.Fatal("equal states read as different")
	}
}

func TestTooMany(t *testing.T) {
	if _, err := Build(Spec{Kind: Revoke}, make([]Candidate, MaxGrants+1), AxisFacts{}); err != ErrTooMany {
		t.Fatalf("err = %v", err)
	}
}
