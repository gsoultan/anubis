package scopedomain_test

import (
	"testing"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

// TestSilenceMeansDeny is the decision, asserted.
//
// An axis exists to narrow access. One created with no effect stated has not
// been reasoned about, and the safe reading of silence is the one that refuses.
// The permissive behaviour is still available — by name.
func TestSilenceMeansDeny(t *testing.T) {
	got, err := scopedomain.NormaliseDefaultEffect("")
	if err != nil {
		t.Fatal(err)
	}
	if got != scopedomain.EffectDeny {
		t.Fatalf("an unstated effect gave %q, want %q — a new axis must not quietly widen every grant that omits it",
			got, scopedomain.EffectDeny)
	}
}

func TestBothEffectsArePreservedWhenStated(t *testing.T) {
	for _, want := range []string{scopedomain.EffectDeny, scopedomain.EffectUnconstrained} {
		got, err := scopedomain.NormaliseDefaultEffect(want)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%q became %q", want, got)
		}
	}
}

// TestATypoIsRefusedByNameNotByConstraint.
//
// The database's CHECK would catch these, and answer with a raw constraint
// violation naming neither the field nor the two values that would have worked.
func TestATypoIsRefusedByNameNotByConstraint(t *testing.T) {
	for _, bad := range []string{"Deny", "DENY", "strict", "allow", "unconstrainted", " deny"} {
		if _, err := scopedomain.NormaliseDefaultEffect(bad); err == nil {
			t.Fatalf("%q was accepted as a default effect", bad)
		}
	}
}
