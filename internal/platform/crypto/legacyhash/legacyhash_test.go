package legacyhash

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// A published bcrypt vector (the one golang.org/x/crypto pins itself to):
// "allmine" at cost 10. If this fails the library underneath changed meaning.
const (
	vectorPassword = "allmine"
	vectorHash     = "$2a$10$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga"
)

func TestVerifyAgainstAPublishedVector(t *testing.T) {
	ok, err := Verify(vectorPassword, vectorHash)
	if err != nil || !ok {
		t.Fatalf("published vector did not verify: ok=%v err=%v", ok, err)
	}
	ok, err = Verify("allmine-but-not-quite", vectorHash)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

// The system being migrated from stores $2a$ at cost 4 and one row of $2y$.
// Both frames must verify, because the importer will see both.
func TestEveryBcryptFrameVerifies(t *testing.T) {
	raw, err := bcrypt.GenerateFromPassword([]byte("moving-day-1234"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"$2a$", "$2b$", "$2y$"} {
		h := prefix + strings.TrimPrefix(string(raw), "$2a$")
		if alg, err := Describe(h); err != nil || alg != AlgBcrypt {
			t.Fatalf("%s: Describe = %q, %v", prefix, alg, err)
		}
		ok, err := Verify("moving-day-1234", h)
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", prefix, ok, err)
		}
	}
}

// A string that is not a bcrypt hash must be refused as such, never verified
// as "wrong password": the importer relies on the distinction to reject a
// pasted plaintext or a PBKDF2 string, and the KDF relies on it to keep
// handling its own format.
func TestNotLegacyIsAnError(t *testing.T) {
	for _, s := range []string{
		"",
		"password",
		"$pbkdf2-sha256$i=600000$c2FsdA$ZGs",
		"$2a$10$tooshort",
		"$2z$10$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga",
		vectorHash + "x",
	} {
		if IsLegacy(s) {
			t.Errorf("%q reported as legacy", s)
		}
		if _, err := Describe(s); err == nil {
			t.Errorf("%q described without error", s)
		}
		if _, err := Verify("anything", s); err == nil {
			t.Errorf("%q verified without error", s)
		}
	}
}
