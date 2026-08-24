package schema

import "strings"

// HeaderIndex maps a sheet's column keys to where they actually sit in the
// header row of the uploaded file. Reading by header rather than by
// position is what lets a workbook still import after an operator has
// reordered the columns, inserted a working column of their own, or
// retyped a header with different capitalisation.
type HeaderIndex struct {
	pos map[string]int
}

// Index locates spec's columns in header. It returns the index plus the
// keys of any required column that is missing, which is a sheet-level
// problem worth reporting once rather than on all four thousand rows.
func Index(spec SheetSpec, header []string) (HeaderIndex, []string) {
	at := make(map[string]int, len(header))
	// tight matches with every separator removed, so "User Name" and
	// "user-name" find the username column too. No two column keys in
	// this schema collide once separators are dropped, so the fallback
	// cannot pick the wrong column.
	tight := make(map[string]int, len(header))
	for i, h := range header {
		n := normalize(h)
		if n == "" {
			continue
		}
		// First column wins: a duplicated header is the operator's copy,
		// and silently preferring the later one would import the wrong
		// column with no indication anything happened.
		if _, seen := at[n]; !seen {
			at[n] = i
		}
		if t := strip(n); t != "" {
			if _, seen := tight[t]; !seen {
				tight[t] = i
			}
		}
	}
	idx := HeaderIndex{pos: make(map[string]int, len(spec.Columns))}
	var missing []string
	for _, c := range spec.Columns {
		i, ok := at[normalize(c.Key)]
		if !ok {
			i, ok = tight[strip(normalize(c.Key))]
		}
		if !ok {
			if c.Required {
				missing = append(missing, c.Key)
			}
			continue
		}
		idx.pos[c.Key] = i
	}
	return idx, missing
}

// Has reports whether the uploaded sheet carried this column at all.
func (h HeaderIndex) Has(key string) bool {
	_, ok := h.pos[key]
	return ok
}

// Value reads a cell by column key, trimmed. A row that stops short of a
// column reads as empty rather than panicking: spreadsheets store rows
// ragged, so a trailing blank simply is not there.
func (h HeaderIndex) Value(row []string, key string) string {
	i, ok := h.pos[key]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// Empty reports whether every column this sheet cares about is blank, so
// the parser can skip the trailing filler rows spreadsheets accumulate
// instead of reporting each one as an error.
func (h HeaderIndex) Empty(row []string) bool {
	for _, i := range h.pos {
		if i < len(row) && strings.TrimSpace(row[i]) != "" {
			return false
		}
	}
	return true
}

// normalize folds a header to its comparison form: case, surrounding
// space, and the difference between "external ref", "External-Ref" and
// "external_ref" all stop mattering.
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range strings.TrimSpace(strings.ToLower(s)) {
		switch {
		case r == ' ' || r == '-' || r == '_':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		default:
			b.WriteRune(r)
			lastUnderscore = false
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}

// strip removes the separators normalize leaves in place.
func strip(s string) string { return strings.ReplaceAll(s, "_", "") }

func equalFold(a, b string) bool { return normalize(a) == normalize(b) }
