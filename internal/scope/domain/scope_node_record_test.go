package scopedomain

import "testing"

func TestPageTokenRoundTrip(t *testing.T) {
	for _, r := range []ScopeNodeRecord{
		{Name: "Office 1", ID: "01a04650-a092-70c3-ba4c-10a1518aa522"},
		{Name: "", ID: "01a04650-a092-70c3-ba4c-10a1518aa523"},
		{Name: "name with = and / and +", ID: "01a04650-a092-70c3-ba4c-10a1518aa524"},
		{Name: "unicode ünïcode 名前", ID: "01a04650-a092-70c3-ba4c-10a1518aa525"},
	} {
		name, id, err := ParsePageToken(r.PageToken())
		if err != nil {
			t.Fatalf("%q: %v", r.Name, err)
		}
		if name != r.Name || id != r.ID {
			t.Errorf("round trip = (%q,%q), want (%q,%q)", name, id, r.Name, r.ID)
		}
	}
}

// A name containing the separator must not be able to shift the id field —
// otherwise a node could be named such that its token resumes somewhere else.
func TestPageTokenSeparatorInName(t *testing.T) {
	r := ScopeNodeRecord{Name: "evil\x00injected", ID: "01a04650-a092-70c3-ba4c-10a1518aa522"}
	name, id, err := ParsePageToken(r.PageToken())
	if err != nil {
		t.Fatal(err)
	}
	if id != r.ID {
		t.Errorf("id = %q, want %q — a name containing the separator moved the cursor", id, r.ID)
	}
	if name != r.Name {
		t.Errorf("name = %q, want %q", name, r.Name)
	}
}

func TestEmptyTokenStartsAtTheBeginning(t *testing.T) {
	name, id, err := ParsePageToken("")
	if err != nil || name != "" || id != "" {
		t.Fatalf("empty token = (%q,%q,%v), want a clean start", name, id, err)
	}
}

// A corrupted cursor must fail loudly. Restarting from page 1 would make a
// paging client loop over the first page forever without ever erroring.
func TestMalformedTokenIsRejected(t *testing.T) {
	for _, tok := range []string{"not base64!!", "", "cGxhaW4"} {
		_, _, err := ParsePageToken(tok)
		if tok == "" {
			continue // empty is legitimately "start at the beginning"
		}
		if err == nil {
			t.Errorf("token %q was accepted", tok)
		}
	}
}

func TestLimitIsClamped(t *testing.T) {
	for _, tc := range []struct{ in, want int32 }{
		{0, DefaultScopeNodePage},
		{-1, DefaultScopeNodePage},
		{10, 10},
		{MaxScopeNodePage + 1, MaxScopeNodePage},
		{1 << 30, MaxScopeNodePage},
	} {
		if got := (ScopeNodeFilter{Limit: tc.in}).Normalise().Limit; got != tc.want {
			t.Errorf("Normalise(limit=%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The default must not be below the historical hard LIMIT 2000: callers that
// send no page_size would otherwise get a SHORTER first page than before,
// which is the same silent truncation this change exists to remove.
func TestDefaultPageIsNotSmallerThanTheOldHardLimit(t *testing.T) {
	if DefaultScopeNodePage < 2000 {
		t.Errorf("DefaultScopeNodePage = %d; callers that send no page_size would regress", DefaultScopeNodePage)
	}
}
