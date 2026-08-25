package secret

import (
	"strings"
	"testing"
)

// Every key this package mints must parse back to the lookup it was stored
// under. This is a property, not an example: the prefix is base64url and
// that alphabet includes '_' and '-', so a parser that searches for a
// separator rather than using the known width fails on roughly one key in
// eight. Those keys authenticated never — they were minted, handed to a
// customer, and rejected as "unauthenticated" forever.
//
// 2,000 keys makes a 12% failure rate a certainty rather than a coin flip;
// the bug reached CI precisely because a single run only had a one-in-eight
// chance of showing it.
func TestEveryMintedAPIKeyRoundTrips(t *testing.T) {
	underscored := 0
	for i := 0; i < 2000; i++ {
		full, lookup, hash, err := NewAPIKey()
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		gotLookup, gotSecret, ok := SplitAPIKey(full)
		if !ok {
			t.Fatalf("key %q did not parse — it could never authenticate", full)
		}
		if gotLookup != lookup {
			t.Fatalf("lookup mismatch: stored %q, parsed %q (key %q)", lookup, gotLookup, full)
		}
		if !Equal(Hash(gotSecret), hash) {
			t.Fatalf("secret half does not hash to the stored value (key %q)", full)
		}
		if strings.Contains(strings.TrimPrefix(lookup, "anb_live_"), "_") {
			underscored++
		}
	}
	// Proves the test actually exercised the case that used to break, rather
	// than passing because no awkward prefix happened to come up.
	if underscored == 0 {
		t.Fatal("no prefix containing '_' was generated — the regression case went untested")
	}
	t.Logf("%d/2000 prefixes contained '_' (each one a key that used to be dead on arrival)", underscored)
}
