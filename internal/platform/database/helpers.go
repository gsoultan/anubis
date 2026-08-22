package database

import "time"

// Column helpers shared by every context adapter: sqlc types nullable
// columns as pointers, and empty strings stand in for SQL NULL on input.

func Deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func DerefS(s *string) string { return Deref(s) }

func DerefBool(b *bool) bool { return b != nil && *b }

func OptStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func OptTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

func OrEmptyJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}

func OrDefaultStr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func OrDefaultInt(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func OrDefaultJSON(b []byte, d string) []byte {
	if len(b) == 0 {
		return []byte(d)
	}
	return b
}

func EmptyIfNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// AuditIP tolerates either representation sqlc infers for host(ip)::text.
func AuditIP(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case *string:
		if t != nil {
			return *t
		}
	}
	return ""
}
