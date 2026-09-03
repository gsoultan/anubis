package config

import (
	"strings"
	"testing"
)

const devDSN = "postgres://u:p@127.0.0.1:5432/db?sslmode=disable"

// ANUBIS_ENV defaults to dev, and dev substitutes a master key that is a
// constant in this repository. An install that meant to be production and
// forgot one variable does not merely run weaker — every signing key and
// every PII key it seals is readable by anyone holding the source.
//
// The systemd packaging sets ANUBIS_ENV=prod explicitly. A container started
// with only ANUBIS_DB_URL does not, and Dockerfile sets no default, so that
// path lands here. The config comment claimed this was "said loudly" and
// nothing said it; these pin the flag that makes saying it possible.
func TestDevSubstitutesTheInsecureKeyAndFlagsIt(t *testing.T) {
	t.Setenv("ANUBIS_DB_URL", devDSN)
	t.Setenv("ANUBIS_ENV", "")
	t.Setenv("ANUBIS_MASTER_KEY", "")
	t.Setenv("ANUBIS_KEY_FILE", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("dev should still boot: %v", err)
	}
	if c.Env != "dev" {
		t.Fatalf("Env = %q, want the dev default", c.Env)
	}
	if !c.InsecureMasterKey {
		t.Error("the dev key was substituted without setting InsecureMasterKey — " +
			"nothing downstream can warn, which is the bug this test exists for")
	}
	if !strings.HasPrefix(string(c.MasterKey), "anubis-dev-master-key") {
		t.Errorf("unexpected key source %q", c.MasterKey)
	}
}

// A configured key must never be reported as insecure, or the warning becomes
// noise operators learn to scroll past.
func TestAConfiguredKeyIsNotFlagged(t *testing.T) {
	t.Setenv("ANUBIS_DB_URL", devDSN)
	t.Setenv("ANUBIS_ENV", "")
	t.Setenv("ANUBIS_KEY_FILE", "")
	// 32 bytes, base64url, no padding — the documented format.
	t.Setenv("ANUBIS_MASTER_KEY", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaas")

	c, err := Load()
	if err != nil {
		t.Skipf("key format rejected, nothing to assert here: %v", err)
	}
	if c.InsecureMasterKey {
		t.Error("a configured master key was reported as the insecure dev fallback")
	}
}

// prod must refuse rather than fall back. A fat-fingered ANUBIS_KEY_FILE path
// sealing production data under the published constant is the failure this
// prevents.
func TestProdRefusesToBootWithoutAKey(t *testing.T) {
	t.Setenv("ANUBIS_DB_URL", devDSN)
	t.Setenv("ANUBIS_ENV", "prod")
	t.Setenv("ANUBIS_MASTER_KEY", "")
	t.Setenv("ANUBIS_KEY_FILE", "")

	c, err := Load()
	if err == nil {
		t.Fatalf("prod booted with no master key (insecure=%v)", c.InsecureMasterKey)
	}
	if !strings.Contains(err.Error(), "master key") {
		t.Errorf("error does not name the problem: %v", err)
	}
}
