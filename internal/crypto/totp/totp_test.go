package totp

import (
	"testing"
	"time"
)

// RFC 6238 Appendix B test vectors: SHA-1, 8 digits, secret
// "12345678901234567890". If these fail the implementation does not speak
// TOTP and no authenticator app will agree with us.
func TestRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	cases := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, c := range cases {
		got := Generate(secret, time.Unix(c.unix, 0).UTC(), DefaultStep, 8)
		if got != c.want {
			t.Errorf("t=%d: got %s want %s", c.unix, got, c.want)
		}
	}
}

func TestVerifySkew(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111111, 0).UTC()

	// exact step
	if _, ok := Verify(secret, "14050471", now, DefaultStep, 8, 1); !ok {
		t.Fatal("exact code rejected")
	}
	// previous step within skew=1 (t=1111111109 sits in the prior window)
	if _, ok := Verify(secret, "07081804", now, DefaultStep, 8, 1); !ok {
		t.Fatal("previous-step code inside skew rejected")
	}
	// two steps back must be outside skew=1
	older := Generate(secret, now.Add(-2*DefaultStep), DefaultStep, 8)
	if _, ok := Verify(secret, older, now, DefaultStep, 8, 1); ok {
		t.Fatal("code two steps old accepted with skew=1")
	}
	// wrong code
	if _, ok := Verify(secret, "00000000", now, DefaultStep, 8, 1); ok {
		t.Fatal("wrong code accepted")
	}
	// wrong length fails fast but safely
	if _, ok := Verify(secret, "1234", now, DefaultStep, 8, 1); ok {
		t.Fatal("short code accepted")
	}
}

func TestMatchedStepEnablesReplayGuard(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(2000000000, 0).UTC()
	code := Generate(secret, now, DefaultStep, 6)

	step1, ok := Verify(secret, code, now, DefaultStep, 6, 1)
	if !ok {
		t.Fatal("fresh code rejected")
	}
	// The same code verifies again at the crypto layer — the REPLAY GUARD is
	// the caller persisting matchedStep and requiring strictly-greater. This
	// test pins the contract that matchedStep is stable for that purpose.
	step2, ok := Verify(secret, code, now, DefaultStep, 6, 1)
	if !ok || step1 != step2 {
		t.Fatalf("matchedStep unstable: %d vs %d", step1, step2)
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI([]byte("12345678901234567890"), "Anubis", "alice", 6, DefaultStep)
	for _, want := range []string{
		"otpauth://totp/Anubis:alice?",
		"algorithm=SHA1", "digits=6", "period=30",
		"secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	} {
		if !contains(uri, want) {
			t.Errorf("uri missing %q: %s", want, uri)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
