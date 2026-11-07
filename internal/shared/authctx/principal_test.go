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

// A person's own sign-in may ask what THEY may do and nothing about anybody
// else; the tenant's systems and the installation's operators may ask about
// anyone, because answering for others is what the decision API is for.
func TestOnlyAPersonsOwnAccessIsAnswerableWithTheirToken(t *testing.T) {
	person := &Principal{IdentityID: "rudi", TenantID: "t1", SessionID: "s1"}
	if !person.MayAskAbout("rudi") {
		t.Error("a person could not ask about themselves")
	}
	if person.MayAskAbout("dewi") {
		t.Error("a person's sign-in could ask about somebody else")
	}
	for name, p := range map[string]*Principal{
		"tenant API key":         {IdentityID: "key-1", TenantID: "t1", Service: true},
		"client-credentials app": {IdentityID: "app_billing", TenantID: "t1", AMR: []string{"client_secret"}},
		"operator":               {IdentityID: "op-1", TenantID: "t1", Platform: true, SessionID: "ps1"},
	} {
		if !p.MayAskAbout("dewi") {
			t.Errorf("%s could not ask about a subject", name)
		}
	}
}
