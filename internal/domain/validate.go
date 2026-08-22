package domain

import "regexp"

var (
	slugRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}$`)
	codeRe     = regexp.MustCompile(`^[a-z][a-z0-9_]{1,30}$`)
	usernameRe = regexp.MustCompile(`^[^\s]{2,128}$`)
)

func ValidSlug(s string) bool     { return slugRe.MatchString(s) }
func ValidCode(s string) bool     { return codeRe.MatchString(s) }
func ValidUsername(s string) bool { return usernameRe.MatchString(s) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
