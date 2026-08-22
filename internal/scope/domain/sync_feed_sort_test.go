package scopedomain

import "testing"

func refs(rows []SyncFeedRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Ref
	}
	return out
}

func assertParentsFirst(t *testing.T, rows []SyncFeedRow) {
	t.Helper()
	seen := map[string]bool{}
	present := map[string]bool{}
	for _, r := range rows {
		present[r.Ref] = true
	}
	for _, r := range rows {
		if r.ParentRef != "" && present[r.ParentRef] && !seen[r.ParentRef] {
			t.Fatalf("child %q emitted before parent %q: %v", r.Ref, r.ParentRef, refs(rows))
		}
		seen[r.Ref] = true
	}
}

// The ERP ordering that broke the first live sync: ORDER BY parent_id puts
// CC-110/CC-120 (parent CC-100) ahead of CC-100 itself (parent CC-ROOT).
func TestSortFixesParentAfterChild(t *testing.T) {
	in := []SyncFeedRow{
		{Ref: "CC-ROOT", Name: "All"},
		{Ref: "CC-110", ParentRef: "CC-100", Name: "Logistics"},
		{Ref: "CC-120", ParentRef: "CC-100", Name: "Manufacturing"},
		{Ref: "CC-100", ParentRef: "CC-ROOT", Name: "Operations"},
		{Ref: "CC-200", ParentRef: "CC-ROOT", Name: "Commercial"},
	}
	got := SortFeedParentsFirst(in)
	if len(got) != len(in) {
		t.Fatalf("lost rows: %v", refs(got))
	}
	assertParentsFirst(t, got)
}

func TestSortKeepsUnknownParentsAndCycles(t *testing.T) {
	// Parent not in the feed: kept (it may already exist in Anubis).
	got := SortFeedParentsFirst([]SyncFeedRow{{Ref: "A", ParentRef: "ELSEWHERE"}})
	if len(got) != 1 || got[0].Ref != "A" {
		t.Fatalf("dropped row with external parent: %v", refs(got))
	}

	// A cycle must not starve output or loop forever.
	cyc := SortFeedParentsFirst([]SyncFeedRow{
		{Ref: "A", ParentRef: "B"},
		{Ref: "B", ParentRef: "A"},
		{Ref: "C"},
	})
	if len(cyc) != 3 {
		t.Fatalf("cycle lost rows: %v", refs(cyc))
	}

	// Self-parent is a degenerate cycle.
	self := SortFeedParentsFirst([]SyncFeedRow{{Ref: "A", ParentRef: "A"}})
	if len(self) != 1 {
		t.Fatalf("self-parent lost: %v", refs(self))
	}
}

func TestSortDeepChainAndStability(t *testing.T) {
	// Reverse-ordered 200-deep chain: every parent must still precede.
	var in []SyncFeedRow
	for i := 200; i >= 1; i-- {
		row := SyncFeedRow{Ref: string(rune('a'+i%26)) + itoa(i)}
		if i > 1 {
			row.ParentRef = string(rune('a'+(i-1)%26)) + itoa(i-1)
		}
		in = append(in, row)
	}
	got := SortFeedParentsFirst(in)
	if len(got) != len(in) {
		t.Fatalf("deep chain lost rows: %d -> %d", len(in), len(got))
	}
	assertParentsFirst(t, got)

	if len(SortFeedParentsFirst(nil)) != 0 {
		t.Fatal("nil input must stay empty")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
