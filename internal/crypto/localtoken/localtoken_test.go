package localtoken

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

func secret(t *testing.T) []byte {
	t.Helper()
	s := make([]byte, 32)
	if _, err := rand.Read(s); err != nil {
		t.Fatal(err)
	}
	return s
}

type mfaState struct {
	IdentityID string `json:"identity_id"`
	DeviceFP   string `json:"device_fp"`
}

func TestRoundTrip(t *testing.T) {
	sec := secret(t)
	now := time.Now()
	tok, err := Seal(sec, "lk1", "mfa", "jti-1", mfaState{"usr_1", "fp"}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "anb.local.v1.") {
		t.Fatalf("prefix: %s", tok)
	}
	kid, err := Kid(tok)
	if err != nil || kid != "lk1" {
		t.Fatalf("kid=%q err=%v", kid, err)
	}
	jti, data, err := Open(sec, tok, "mfa", now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if jti != "jti-1" || !bytes.Contains(data, []byte("usr_1")) {
		t.Fatalf("jti=%q data=%s", jti, data)
	}
}

func TestPurposeIsBoundByAEAD(t *testing.T) {
	// An MFA token presented to the password-reset opener must fail
	// AUTHENTICATION (AAD mismatch), not merely a field comparison.
	sec := secret(t)
	now := time.Now()
	tok, _ := Seal(sec, "lk1", "mfa", "j", mfaState{}, time.Minute, now)
	if _, _, err := Open(sec, tok, "password_reset", now); err != ErrOpen {
		t.Fatalf("want ErrOpen (AAD mismatch), got %v", err)
	}
}

func TestExpiry(t *testing.T) {
	sec := secret(t)
	now := time.Now()
	tok, _ := Seal(sec, "lk1", "mfa", "j", mfaState{}, time.Minute, now)
	if _, _, err := Open(sec, tok, "mfa", now.Add(2*time.Minute)); err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestWrongKeyAndTamper(t *testing.T) {
	sec := secret(t)
	now := time.Now()
	tok, _ := Seal(sec, "lk1", "mfa", "j", mfaState{}, time.Minute, now)

	if _, _, err := Open(secret(t), tok, "mfa", now); err != ErrOpen {
		t.Fatalf("wrong key: want ErrOpen, got %v", err)
	}

	raw := []byte(tok)
	raw[len(raw)-1] ^= 1
	if _, _, err := Open(sec, string(raw), "mfa", now); err == nil {
		t.Fatal("tampered token opened")
	}
}

func TestMalformed(t *testing.T) {
	sec := secret(t)
	for _, tok := range []string{
		"", "anb.local.v1.", "anb.local.v1.a", "anb.local.v1..b",
		"anb.local.v1.a.b.c", "anb.local.v2.a.b", "v4.public.xx",
		"anb.local.v1.!!!.AAAA", "anb.local.v1.a2lk.AAAA",
	} {
		if _, _, err := Open(sec, tok, "mfa", time.Now()); err == nil {
			t.Errorf("malformed %q opened", tok)
		}
	}
}

func FuzzOpen(f *testing.F) {
	sec := make([]byte, 32)
	tok, _ := Seal(sec, "k", "mfa", "j", mfaState{"a", "b"}, time.Minute, time.Now())
	f.Add(tok)
	f.Add("anb.local.v1.a.b")
	f.Fuzz(func(t *testing.T, s string) {
		// must never panic
		_, _, _ = Open(sec, s, "mfa", time.Now())
		_, _ = Kid(s)
	})
}
