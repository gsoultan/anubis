package accesstoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/gsoultan/anubis/pkg/anubis/jws"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// Every call site that used to branch on a "v4.public." prefix now goes
// through here, so both formats must behave identically for all of it. When
// they did not, enabling JWS for one application would have made the gate
// deny, introspection report inactive and revoke do nothing — three
// independent silent failures from one flag.
func TestBothFormatsBehaveTheSame(t *testing.T) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	claims := []byte(`{"sub":"alice","sid":"s1","tid":"acme"}`)

	footer, _ := json.Marshal(map[string]string{"kid": "k1"})
	pasetoTok, err := paseto.Sign(sk, claims, footer, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwsTok, err := jws.Sign(sk, claims, "k1")
	if err != nil {
		t.Fatal(err)
	}

	for name, tok := range map[string]string{"paseto": pasetoTok, "jws": jwsTok} {
		t.Run(name, func(t *testing.T) {
			if !IsAccessToken(tok) {
				t.Fatal("not recognised as an access token")
			}
			kid, err := Kid(tok)
			if err != nil {
				t.Fatal(err)
			}
			if kid != "k1" {
				t.Fatalf("kid came back %q", kid)
			}
			got, err := Verify(pk, tok)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(claims) {
				t.Fatalf("claims came back %q", got)
			}
			un, err := ClaimsUnverified(tok)
			if err != nil {
				t.Fatal(err)
			}
			if string(un) != string(claims) {
				t.Fatalf("unverified claims came back %q", un)
			}
		})
	}
}

// A token signed by the wrong key must fail in BOTH formats. A helper that
// dispatched to a codec which quietly accepted would be worse than two
// branches.
func TestWrongKeyFailsInBothFormats(t *testing.T) {
	pk, _, _ := ed25519.GenerateKey(rand.Reader)
	_, otherSK, _ := ed25519.GenerateKey(rand.Reader)
	claims := []byte(`{"sub":"attacker"}`)

	footer, _ := json.Marshal(map[string]string{"kid": "k1"})
	p, _ := paseto.Sign(otherSK, claims, footer, nil)
	j, _ := jws.Sign(otherSK, claims, "k1")

	for name, tok := range map[string]string{"paseto": p, "jws": j} {
		if _, err := Verify(pk, tok); err == nil {
			t.Fatalf("%s: a token signed by another key verified", name)
		}
	}
}

// An opaque refresh token is not an access token, and must not be mistaken
// for one — revoke dispatches on exactly this.
func TestRefreshTokenIsNotAnAccessToken(t *testing.T) {
	for _, s := range []string{"anb_rt_abc123", "", "garbage", "a.b"} {
		if IsAccessToken(s) {
			t.Fatalf("%q was taken for an access token", s)
		}
	}
}

// alg=none must not survive the dispatch either: the helper picks a CODEC by
// format marker, and the codec it picks still pins EdDSA.
func TestAlgNoneIsRejectedThroughTheHelper(t *testing.T) {
	pk, _, _ := ed25519.GenerateKey(rand.Reader)
	head := `{"alg":"none","typ":"JWT","kid":"k1"}`
	b := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	tok := b(head) + "." + b(`{"sub":"attacker"}`) + "." + b(string(make([]byte, 64)))
	if _, err := Verify(pk, tok); err == nil {
		t.Fatal("alg=none verified")
	}
}
