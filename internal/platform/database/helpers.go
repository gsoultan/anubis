package database

import "time"

// Column helpers shared by every context adapter.
//
// These are INPUT-side now. The output side used to need them too — sqlc typed
// every nullable column as a pointer, so each read dereferenced one — and
// storm reads a nullable column as runtime.Null[T], which the adapters unwrap
// with their own two-line helper. Deref, DerefS, DerefBool and AuditIP went
// with sqlc; nothing had called them since the last context migrated.

// OptStr turns an absent string into SQL NULL. Empty is not the same as
// absent for a nullable column: ” is a value a unique index will collide on,
// and NULL is the absence a partial index is written for.
func OptStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// OptTime treats the zero time as absent, which is what the domain means by it.
func OptTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

// OrEmptyJSON keeps a jsonb column valid when the caller has nothing to say.
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

// EmptyIfNil sends '{}' rather than NULL for a text[] column: the columns
// these feed are NOT NULL with a '{}' default, and a nil slice would write
// NULL over it.
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
