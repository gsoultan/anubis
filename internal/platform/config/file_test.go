package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func sample() *FileConfig {
	return &FileConfig{
		DBHost: "db.internal", DBPort: 5432, DBName: "anubis", DBUser: "anubis",
		DBPassword: `p@ss:w/rd #1 "quoted"`, DBSSLMode: "require",
		Listen: ":7448", Issuer: "https://id.example.com", UIOrigin: "https://console.example.com",
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	master := key(t)
	want := sample()
	if err := want.Save(path, master); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path, master)
	if err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Fatalf("round trip changed the config:\n got %+v\nwant %+v", got, want)
	}
}

// The password is the whole reason this file is sealed. It must not appear.
func TestPasswordIsNotWrittenInClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := sample().Save(path, key(t)); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "p@ss") {
		t.Fatalf("the password is in the file:\n%s", body)
	}
	if !strings.Contains(string(body), "enc:v1:") {
		t.Error("the password does not look sealed")
	}
}

// A config file readable by everyone on the host defeats sealing it.
func TestConfigIsWrittenPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := sample().Save(path, key(t)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestWrongMasterKeyIsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := sample().Save(path, key(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, key(t)); err == nil {
		t.Fatal("a different master key should not open the config")
	}
}

// A plaintext password would make sealing optional in practice, and nobody
// would notice which installations had skipped it.
func TestPlaintextPasswordIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "database:\n  host: localhost\n  name: anubis\n  user: anubis\n  password: hunter2\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path, key(t))
	if err == nil || !strings.Contains(err.Error(), "not sealed") {
		t.Fatalf("err = %v, want a not-sealed error", err)
	}
}

// One '@' or '/' in a generated password would otherwise produce a DSN
// pointing at an entirely different host.
func TestDatabaseURLEscapesTheCredentials(t *testing.T) {
	c := sample()
	c.DBPassword = "p@ss/word?x=1"
	got := c.DatabaseURL()
	if strings.Contains(got, "p@ss/word") {
		t.Fatalf("password not escaped: %s", got)
	}
	if !strings.HasPrefix(got, "postgres://anubis:") || !strings.Contains(got, "@db.internal:5432/anubis") {
		t.Fatalf("dsn = %s", got)
	}
	if !strings.Contains(got, "sslmode=require") {
		t.Errorf("sslmode missing: %s", got)
	}
}

func TestSealRejectsTamperedValues(t *testing.T) {
	master := key(t)
	sealed, err := Seal(master, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Open(master, sealed); err != nil || got != "secret" {
		t.Fatalf("round trip: %q %v", got, err)
	}
	// Flip a byte of the CIPHERTEXT itself. Tampering with the last base64
	// character instead would prove nothing: the final character carries
	// spare bits that decode away, so it can round-trip unchanged.
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(sealed, sealPrefix))
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	tampered := sealPrefix + base64.RawURLEncoding.EncodeToString(raw)
	if _, err := Open(master, tampered); err == nil {
		t.Error("a tampered ciphertext should not open")
	}
	// A truncated blob must not panic its way through the nonce split.
	if _, err := Open(master, sealPrefix+base64.RawURLEncoding.EncodeToString(raw[:4])); err == nil {
		t.Error("a truncated sealed value should not open")
	}
	if _, err := Open(master, "not-sealed"); err != ErrNotSealed {
		t.Errorf("err = %v, want ErrNotSealed", err)
	}
}

// The installer's gate. A stale answer either keeps the installer shut on an
// install that has none, or reopens it on one that is configured.
func TestConfiguredFollowsTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("ANUBIS_CONFIG", path)

	if Configured() {
		t.Fatal("no file, yet reported configured")
	}
	if err := sample().Save(path, key(t)); err != nil {
		t.Fatal(err)
	}
	if !Configured() {
		t.Fatal("file written, yet reported unconfigured")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if Configured() {
		t.Fatal("file removed, yet still reported configured")
	}
}

func TestEnsureMasterKeyPrefersTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANUBIS_CONFIG", filepath.Join(dir, "config.yaml"))
	t.Setenv("ANUBIS_KEY_FILE", filepath.Join(dir, "anubis.key"))
	t.Setenv("ANUBIS_MASTER_KEY", "")

	k1, generated, err := EnsureMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Error("the first call should have generated a key")
	}
	info, err := os.Stat(filepath.Join(dir, "anubis.key"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file mode = %o, want 600", perm)
	}
	// A second call must reuse it, or every restart would orphan the config.
	k2, generated, err := EnsureMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Error("the second call regenerated the key")
	}
	if string(k1) != string(k2) {
		t.Error("the key changed between calls")
	}
}

// A key file is how a systemd credential reaches the process: mounted 0400
// into a private tmpfs at a path that changes every boot, which is why the
// path has to be configurable rather than fixed. Before this worked, prod
// refused to boot with "ANUBIS_MASTER_KEY is required" even when a perfectly
// good key file was named.
func TestMasterKeyFromFileSatisfiesProd(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "master.key")
	// 32 bytes, base64url, as the runbook mints it.
	if err := os.WriteFile(keyPath, []byte(base64.RawURLEncoding.EncodeToString(
		bytes.Repeat([]byte{7}, 32))), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUBIS_MASTER_KEY", "")
	t.Setenv("ANUBIS_KEY_FILE", keyPath)
	t.Setenv("ANUBIS_ENV", "prod")
	t.Setenv("ANUBIS_DB_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("ANUBIS_ISSUER", "https://auth.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("prod refused a file-based master key: %v", err)
	}
	if !bytes.Equal(cfg.MasterKey, bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("loaded a different key than the file holds")
	}
}

// A key source that is NAMED but broken must stop the process. Falling back
// to the dev key here would seal production data under a key printed in the
// source of this repository.
func TestBrokenMasterKeySourceIsFatalEvenInDev(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "master.key")
	if err := os.WriteFile(keyPath, []byte("not-a-32-byte-base64url-key"), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUBIS_MASTER_KEY", "")
	t.Setenv("ANUBIS_KEY_FILE", keyPath)
	t.Setenv("ANUBIS_ENV", "dev")
	t.Setenv("ANUBIS_DB_URL", "postgres://u:p@localhost:5432/db")

	if _, err := Load(); err == nil {
		t.Fatal("a malformed key file was ignored and the dev key used instead")
	}
}
