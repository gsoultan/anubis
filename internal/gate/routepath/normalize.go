// Package gate matches request paths against route policies. Path
// normalisation IS the security-critical part (ADR-0006): the gap between
// two normalisers is the bypass, so this one is shared — the gate and any
// in-app router must both call NormalizePath, and the corpus in
// testdata/normalize_corpus.txt runs against every consumer.
package routepath

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrAmbiguousPath = errors.New("gate: ambiguous path rejected")

// NormalizePath canonicalises a request path for matching. Fail-closed: any
// input this cannot confidently normalise is an error, and an error at the
// gate is a deny.
//
// Rules:
//   - must start with '/'
//   - exactly ONE round of percent-decoding; a '%' surviving decode (double
//     encoding) is ambiguous -> reject
//   - NUL, control bytes, and backslashes are ambiguous -> reject
//   - ';' (path-parameter trick) is ambiguous -> reject
//   - a DECODED '#' or '?' is ambiguous -> reject: raw ones delimit the
//     fragment/query and are stripped, so one that appears after decoding
//     (%23/%3F) reads as data here and as a delimiter to any re-parser
//   - '.' and '..' segments resolve; escaping the root -> reject
//   - repeated slashes collapse
//   - a trailing slash is preserved (distinct resources)
func NormalizePath(raw string) (string, error) {
	if raw == "" || raw[0] != '/' {
		return "", ErrAmbiguousPath
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
		if raw == "" {
			raw = "/"
		}
	}

	decoded, err := decodeOnce(raw)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(decoded) {
		return "", ErrAmbiguousPath // overlong/invalid UTF-8 is a classic filter bypass
	}
	// Dot-segments that only appear AFTER decoding were hiding traversal
	// behind percent-encoding; literal ./.. resolve below, encoded ones die.
	encodedDots := strings.Contains(raw, "%2e") || strings.Contains(raw, "%2E")
	for i := 0; i < len(decoded); i++ {
		c := decoded[i]
		if c < 0x20 || c == 0x7f || c == '\\' || c == ';' || c == '%' || c == 0 ||
			c == '#' || c == '?' {
			return "", ErrAmbiguousPath
		}
	}

	trailingSlash := strings.HasSuffix(decoded, "/") && decoded != "/"
	segs := strings.Split(decoded, "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		switch s {
		case "":
			continue
		case ".":
			if encodedDots {
				return "", ErrAmbiguousPath
			}
			continue
		case "..":
			if encodedDots {
				return "", ErrAmbiguousPath // %2e-encoded traversal
			}
			if len(out) == 0 {
				return "", ErrAmbiguousPath // traversal above root
			}
			out = out[:len(out)-1]
		default:
			out = append(out, s)
		}
	}
	norm := "/" + strings.Join(out, "/")
	if trailingSlash && norm != "/" {
		norm += "/"
	}
	return norm, nil
}

func decodeOnce(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", ErrAmbiguousPath
		}
		hi, ok1 := unhex(s[i+1])
		lo, ok2 := unhex(s[i+2])
		if !ok1 || !ok2 {
			return "", ErrAmbiguousPath
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
