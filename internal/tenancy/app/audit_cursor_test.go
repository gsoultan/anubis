package tenancyapp

import "testing"

// The audit page token is a sequence number the client hands back to walk
// older entries. encodeSeq/decodeSeq must round-trip, and decodeSeq must
// reject anything that is not a plain sequence number so a token cannot carry
// something else into the query.
func TestAuditCursorRoundTrips(t *testing.T) {
	for _, n := range []int64{0, 1, 9, 10, 42, 1_000_000, 9_223_372_036_854_775_807} {
		got, err := decodeSeq(encodeSeq(n))
		if err != nil {
			t.Fatalf("decodeSeq(encodeSeq(%d)): %v", n, err)
		}
		if got != n {
			t.Fatalf("round trip of %d gave %d", n, got)
		}
	}
}

func TestAuditCursorRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "-1", "12a", "0x10", " 5", "5 ", "abc", "1.0"} {
		if _, err := decodeSeq(s); err == nil {
			t.Fatalf("decodeSeq(%q) was accepted; a non-numeric cursor must be rejected", s)
		}
	}
}
