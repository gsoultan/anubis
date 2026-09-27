package authctx

import "testing"

func TestFirstParty(t *testing.T) {
	for _, c := range []struct {
		name string
		aud  []string
		want bool
	}{
		{"a token Anubis issued for itself", []string{"anubis"}, true},
		{"a token issued to an application", []string{"billing"}, false},
		{"no audience at all", nil, false},
		{"the platform audience is not first-party self-service", []string{"anubis-platform"}, false},
		{"anubis among several", []string{"billing", "anubis"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := (&Principal{Audience: c.aud}).FirstParty(); got != c.want {
				t.Fatalf("FirstParty(%v) = %v, want %v", c.aud, got, c.want)
			}
		})
	}
}
