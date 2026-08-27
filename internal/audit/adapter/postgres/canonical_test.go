package auditpg

import "testing"

// The bug this pins: detail is a jsonb column, so Postgres re-renders what it
// stores. Hashing the bytes the writer sent produced a hash the reader could
// never reproduce, and 21,424 of 21,439 entries in a real database read as
// tampered. Both sides must hash the same canonical form.
func TestCanonicalJSONSurvivesPostgresReformatting(t *testing.T) {
	cases := []struct{ written, readBack string }{
		// jsonb adds a space after every colon.
		{`{"reason":"invalid_credentials"}`, `{"reason": "invalid_credentials"}`},
		// and after commas, and reorders keys by its own rule.
		{`{"a":1,"b":2}`, `{"b": 2, "a": 1}`},
		{`{"family_id":"01a0"}`, `{"family_id": "01a0"}`},
		// nesting is reformatted too
		{`{"targets":{"org":"x"},"n":3}`, `{"n": 3, "targets": {"org": "x"}}`},
	}
	for _, c := range cases {
		w := string(canonicalJSON([]byte(c.written)))
		r := string(canonicalJSON([]byte(c.readBack)))
		if w != r {
			t.Errorf("writer and reader disagree:\n  wrote %s -> %s\n  read  %s -> %s",
				c.written, w, c.readBack, r)
		}
	}
}

// An absent detail and an empty object cannot be told apart once stored:
// both leave {} in the row. The hash must identify them or every
// detail-less entry fails to verify.
func TestCanonicalJSONTreatsEmptyDetailAsAbsent(t *testing.T) {
	for _, in := range []string{"", "{}", "{ }"} {
		if got := canonicalJSON([]byte(in)); len(got) != 0 {
			t.Errorf("canonicalJSON(%q) = %q, want empty", in, got)
		}
	}
}

// A large integer must not be routed through float64 and come back a
// different number than it went in.
func TestCanonicalJSONKeepsIntegerPrecision(t *testing.T) {
	const big = `{"n":9007199254740993}`
	if got := string(canonicalJSON([]byte(big))); got != big {
		t.Errorf("precision lost: %s -> %s", big, got)
	}
}

// Something that is not JSON should be hashed as it stands rather than
// silently replaced — it should never occur, and hiding it would be worse.
func TestCanonicalJSONPassesThroughNonJSON(t *testing.T) {
	in := []byte("not json at all")
	if got := string(canonicalJSON(in)); got != string(in) {
		t.Errorf("mangled a non-JSON detail: %q", got)
	}
}
