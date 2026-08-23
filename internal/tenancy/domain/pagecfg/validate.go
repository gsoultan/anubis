package pagecfg

import (
	"encoding/json"
	"net/url"
	"strings"
	"unicode"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

const maxURLLength = 512

func invalid(field, why string) error {
	return apperr.ErrInvalidArgument.With("field", field).With("reason", why)
}

// checkText rejects control characters outright. They never appear in real
// copy, and they are how a value smuggles line breaks into places that assume
// one line.
func checkText(field, value string, max int, required bool) error {
	if value == "" {
		if required {
			return invalid(field, "required")
		}
		return nil
	}
	if len([]rune(value)) > max {
		return invalid(field, "too long")
	}
	for _, r := range value {
		if r != '\t' && unicode.IsControl(r) {
			return invalid(field, "control characters are not allowed")
		}
	}
	return nil
}

// validColor allows only #rgb and #rrggbb. Anything else — including CSS that
// happens to be valid, like "red" or "var(--x)" — is refused, because these
// values are interpolated into a stylesheet and the safe set is the one we
// can prove terminates.
func validColor(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// checkURL permits http(s) only. javascript:, data:, vbscript: and friends
// execute in Anubis's origin when clicked from a page Anubis served.
func checkURL(field, raw string) error {
	if len(raw) > maxURLLength {
		return invalid(field, "too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return invalid(field, "not a URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return invalid(field, "only http and https URLs are allowed")
	}
	if u.Host == "" {
		return invalid(field, "missing host")
	}
	return nil
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
