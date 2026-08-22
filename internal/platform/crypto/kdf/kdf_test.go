package kdf

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// Published PBKDF2-HMAC-SHA256 test vectors (the RFC 6070 inputs re-run under
// SHA-256, as used by RFC 7914 §11 and NIST CAVP). If these fail, the
// primitive underneath us changed meaning — stop everything.
func TestPBKDF2SHA256Vectors(t *testing.T) {
	cases := []struct {
		password string
		salt     string
		iter     int
		want     string
	}{
		{"password", "salt", 1,
			"120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"password", "salt", 2,
			"ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{"password", "salt", 4096,
			"c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
	}
	for _, c := range cases {
		dk, err := pbkdf2.Key(sha256.New, c.password, []byte(c.salt), c.iter, 32)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(dk); got != c.want {
			t.Errorf("iter=%d: got %s want %s", c.iter, got, c.want)
		}
	}
}

func TestHashAndVerify(t *testing.T) {
	// Full-strength hashing is exercised once in TestDefaultIterations; the
	// behavioural cases run at low cost via the internal helper.
	h, err := hashWith("correct horse battery staple", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$pbkdf2-sha256$i=1000$") {
		t.Fatalf("format: %s", h)
	}

	ok, rehash, err := Verify("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
	if !rehash {
		t.Error("1000 iterations must demand a rehash under the 600k policy")
	}

	ok, _, err = Verify("wrong password", h)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

func TestDefaultIterations(t *testing.T) {
	if testing.Short() {
		t.Skip("600k-iteration hash")
	}
	h, err := Hash("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h, "$i=600000$") {
		t.Fatalf("default must be 600k: %s", h)
	}
	ok, rehash, err := Verify("s3cret", h)
	if err != nil || !ok || rehash {
		t.Fatalf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "plainhash", "$pbkdf2-sha256$i=600000$onlysalt",
		"$pbkdf2-sha256$x=600000$c2FsdA$c2FsdA",
		"$pbkdf2-sha256$i=0$c2FsdA$c2FsdA",
		"$pbkdf2-sha256$i=99999999$c2FsdA$c2FsdA", // absurd cost = DoS input
		"$scrypt$i=1$c2FsdA$c2FsdA",
		"$pbkdf2-sha256$i=1000$!!$c2FsdA",
	} {
		if ok, _, err := Verify("pw", bad); ok || err == nil {
			t.Errorf("malformed %q: ok=%v err=%v", bad, ok, err)
		}
	}
}

func TestDummyIsStableAndValid(t *testing.T) {
	if Dummy() != Dummy() {
		t.Fatal("dummy hash must be stable within a process")
	}
	// It must parse like any real hash so the verify path is identical.
	if _, _, _, _, err := parse(Dummy()); err != nil {
		t.Fatalf("dummy hash unparseable: %v", err)
	}
}
