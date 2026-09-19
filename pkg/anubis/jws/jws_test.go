package jws

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func keypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pk, sk
}

func TestRoundTrip(t *testing.T) {
	pk, sk := keypair(t)
	payload := []byte(`{"sub":"alice","exp":1}`)

	tok, err := Sign(sk, payload, "k1")
	if err != nil {
		t.Fatal(err)
	}
	got, h, err := Verify(pk, tok)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload came back %q", got)
	}
	if h.Kid != "k1" || h.Alg != "EdDSA" || h.Typ != "JWT" {
		t.Fatalf("header came back %+v", h)
	}
}

// THE reason this package exists rather than a JOSE library. A verifier that
// reads alg from the token it is checking can be told not to check.
func TestAlgNoneIsRejected(t *testing.T) {
	pk, _ := keypair(t)
	for _, alg := range []string{"none", "None", "NONE", "HS256", "RS256", "ES256", ""} {
		head, _ := json.Marshal(map[string]string{"alg": alg, "typ": "JWT"})
		tok := base64.RawURLEncoding.EncodeToString(head) + "." +
			base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"attacker"}`)) + "." +
			base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
		if _, _, err := Verify(pk, tok); err == nil {
			t.Fatalf("alg=%q was accepted", alg)
		}
	}
}

// The other half of algorithm confusion: a valid EdDSA token whose signature
// is stripped or blanked must not verify.
func TestTamperedSignature(t *testing.T) {
	pk, sk := keypair(t)
	tok, err := Sign(sk, []byte(`{"sub":"alice"}`), "k1")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.LastIndexByte(tok, '.')
	blank := tok[:i+1] + base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if _, _, err := Verify(pk, blank); err == nil {
		t.Fatal("a blank signature verified")
	}
}

// A token signed by somebody else's key must not verify, which is the only
// thing kid could ever be trusted to change if kid were read as an
// instruction rather than a hint.
func TestWrongKeyDoesNotVerify(t *testing.T) {
	pk, _ := keypair(t)
	_, otherSK := keypair(t)
	tok, err := Sign(otherSK, []byte(`{"sub":"alice"}`), "k1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Verify(pk, tok); err == nil {
		t.Fatal("a token signed by another key verified")
	}
}

// Changing the payload after signing must fail, including changes that
// base64-decode to the same bytes under a lenient decoder.
func TestPayloadTamperingFails(t *testing.T) {
	pk, sk := keypair(t)
	tok, err := Sign(sk, []byte(`{"sub":"alice","admin":false}`), "k1")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice","admin":true}`))
	if _, _, err := Verify(pk, strings.Join(parts, ".")); err == nil {
		t.Fatal("an edited payload verified")
	}
}

// kid lives in the PROTECTED header, so re-pointing a valid token at another
// key changes the signed bytes and breaks the signature.
func TestKidIsProtected(t *testing.T) {
	pk, sk := keypair(t)
	tok, err := Sign(sk, []byte(`{"sub":"alice"}`), "k1")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	head, _ := json.Marshal(Header{Alg: "EdDSA", Typ: "JWT", Kid: "k2"})
	parts[0] = base64.RawURLEncoding.EncodeToString(head)
	if _, _, err := Verify(pk, strings.Join(parts, ".")); err == nil {
		t.Fatal("kid was swapped without breaking the signature")
	}
}

// RFC 7515 §4.1.11: a verifier that does not implement every extension named
// in crit must reject. Ignoring it makes "critical" advisory.
func TestCritIsRejected(t *testing.T) {
	pk, sk := keypair(t)
	head, _ := json.Marshal(map[string]any{
		"alg": "EdDSA", "typ": "JWT", "crit": []string{"exp"},
	})
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice"}`))
	signing := base64.RawURLEncoding.EncodeToString(head) + "." + payload
	sig := ed25519.Sign(sk, []byte(signing))
	tok := signing + "." + base64.RawURLEncoding.EncodeToString(sig)

	if _, _, err := Verify(pk, tok); err != ErrCritHeader {
		t.Fatalf("crit was not rejected: %v", err)
	}
}

// A compact JWE has five parts. Accepting one would hand the caller an
// encrypted payload as though it were verified claims.
func TestJWEShapeIsRejected(t *testing.T) {
	pk, _ := keypair(t)
	if _, _, err := Verify(pk, "a.b.c.d.e"); err == nil {
		t.Fatal("a five-part token was accepted")
	}
}

func TestGarbageDoesNotPanic(t *testing.T) {
	pk, _ := keypair(t)
	for _, s := range []string{
		"", ".", "..", "...", "a", "a.b", "!!.??.$$",
		strings.Repeat("A", 5000) + ".a.b",
	} {
		if _, _, err := Verify(pk, s); err == nil {
			t.Fatalf("%q verified", s)
		}
	}
}

func FuzzVerify(f *testing.F) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		f.Fatal(err)
	}
	tok, _ := Sign(sk, []byte(`{"sub":"alice"}`), "k1")
	f.Add(tok)
	f.Add("a.b.c")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		// The only contract: never panic, and never return a payload
		// alongside an error.
		payload, _, err := Verify(pk, s)
		if err != nil && payload != nil {
			t.Fatal("payload returned with an error")
		}
	})
}
