// Package totp implements RFC 6238 (over RFC 4226 HOTP) with the standard
// library. Hand-written per ADR-0002: it is a format layer over crypto/hmac.
//
// SHA-1 is used as RFC 6238 specifies for authenticator-app compatibility;
// its known weaknesses (collisions) do not apply to HMAC usage here.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const (
	DefaultDigits = 6
	DefaultStep   = 30 * time.Second
	secretLen     = 20 // 160 bits, RFC 4226 recommendation
)

// hotp computes the RFC 4226 value for one counter.
func hotp(secret []byte, counter uint64, digits int) string {
	mac := hmac.New(sha1.New, secret)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod)
}

// Step returns the time-step counter for t.
func Step(t time.Time, step time.Duration) uint64 {
	return uint64(t.Unix()) / uint64(step/time.Second)
}

// Generate returns the code for time t.
func Generate(secret []byte, t time.Time, step time.Duration, digits int) string {
	return hotp(secret, Step(t, step), digits)
}

// Verify checks code within ±skew steps around t and returns the matching
// step so the caller can enforce monotonic single use (a step may only ever
// be accepted once; store it and require acceptedStep > lastStep).
func Verify(secret []byte, code string, t time.Time, step time.Duration, digits, skew int) (matchedStep uint64, ok bool) {
	if len(code) != digits {
		return 0, false
	}
	base := Step(t, step)
	for d := -skew; d <= skew; d++ {
		s := int64(base) + int64(d)
		if s < 0 {
			continue
		}
		want := hotp(secret, uint64(s), digits)
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return uint64(s), true
		}
	}
	return 0, false
}

// NewSecret generates a fresh shared secret.
func NewSecret() ([]byte, error) {
	s := make([]byte, secretLen)
	if _, err := rand.Read(s); err != nil {
		return nil, err
	}
	return s, nil
}

// ProvisioningURI renders the otpauth:// URI authenticator apps enrol from.
func ProvisioningURI(secret []byte, issuer, account string, digits int, step time.Duration) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	v := url.Values{}
	v.Set("secret", enc)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", strconv.Itoa(digits))
	v.Set("period", strconv.Itoa(int(step/time.Second)))
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}
