package authzcatalog

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// Formats a document can arrive in. JSON is the manifest an application
// posts; CSV is what falls out of every other system in the building.
const (
	FormatJSON = "json"
	FormatCSV  = "csv"
)

// A catalog is a document a human maintains, not a data feed. Ten thousand
// rows is already an application nobody can reason about, and the cap is
// what stops a wrong URL from being a memory incident.
const maxRows = 10_000

// Parse reads a document in the named format. An empty format is JSON, so
// every existing caller keeps working.
func Parse(text, format string) (*Document, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", FormatJSON:
		return parseJSON(text)
	case FormatCSV:
		return parseCSV(text)
	default:
		return nil, apperr.ErrInvalidArgument.
			With("format", format).
			With("expected", FormatJSON+" or "+FormatCSV)
	}
}

// parseJSON detects sections with pointers: a nil slice pointer is a key
// that was never written, which is the whole difference between "leave my
// routes alone" and "delete my routes".
func parseJSON(text string) (*Document, error) {
	var raw struct {
		Permissions *[]Permission `json:"permissions"`
		Roles       *[]Role       `json:"roles"`
		Routes      *[]Route      `json:"routes"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, apperr.ErrInvalidArgument.With("manifest", "invalid JSON").Wrap(err)
	}
	d := &Document{}
	if raw.Permissions != nil {
		d.Permissions, d.HasPermissions = *raw.Permissions, true
	}
	if raw.Roles != nil {
		d.Roles, d.HasRoles = *raw.Roles, true
	}
	if raw.Routes != nil {
		d.Routes, d.HasRoutes = *raw.Routes, true
	}
	return d, nil
}

// parseCSV reads ONE kind of row per file, decided by the header. Two files
// rather than one with a discriminator column: a spreadsheet of permissions
// and a spreadsheet of roles have no columns in common, and a single sheet
// holding both is nine empty cells per row.
//
// Routes are deliberately not expressible. A route carries a scope-binding
// map and an ordering that decides which rule wins; flattening that into
// cells produces a file whose meaning depends on column order, which is
// exactly the document you do not want deciding who gets in.
func parseCSV(text string) (*Document, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	// Excel drops trailing empty cells rather than padding the row, so a
	// ragged record is the normal export, not a malformed file.
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err == io.EOF {
		return nil, apperr.ErrInvalidArgument.With("csv", "empty file")
	}
	if err != nil {
		return nil, apperr.ErrInvalidArgument.With("csv", "unreadable").Wrap(err)
	}
	idx := index(header)

	switch {
	case idx.has("resource") && idx.has("action"):
		return csvPermissions(r, idx)
	case idx.has("permissions") || idx.has("role"):
		return csvRoles(r, idx)
	default:
		return nil, apperr.ErrInvalidArgument.
			With("csv", "header names neither a permission nor a role sheet").
			With("permissions", "resource, action, description, risk, min_assurance, requires_amr, max_auth_age").
			With("roles", "role, description, permissions, patterns, allowed_realm_kinds")
	}
}

func csvPermissions(r *csv.Reader, idx headerIndex) (*Document, error) {
	d := &Document{HasPermissions: true}
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			return d, nil
		}
		if err != nil {
			return nil, apperr.ErrInvalidArgument.With("csv", "unreadable").
				With("line", strconv.Itoa(line)).Wrap(err)
		}
		if blank(rec) {
			continue
		}
		if len(d.Permissions) >= maxRows {
			return nil, apperr.ErrInvalidArgument.With("csv", "too many rows").
				With("max", strconv.Itoa(maxRows))
		}
		p := Permission{
			Resource:    idx.get(rec, "resource"),
			Action:      idx.get(rec, "action"),
			Description: idx.get(rec, "description"),
			Risk:        idx.get(rec, "risk"),
			RequiresAMR: multi(idx.get(rec, "requires_amr")),
			MaxAuthAge:  idx.get(rec, "max_auth_age"),
		}
		// A blank risk column on a spreadsheet means the author did not have
		// an opinion, and the table's own default is the one to inherit.
		if p.Risk == "" {
			p.Risk = "normal"
		}
		if v := idx.get(rec, "min_assurance"); v != "" {
			n, cerr := strconv.Atoi(v)
			if cerr != nil {
				return nil, apperr.ErrInvalidArgument.
					With("line", strconv.Itoa(line)).
					With("min_assurance", v).With("expected", "1, 2 or 3")
			}
			p.MinAssurance = n
		}
		d.Permissions = append(d.Permissions, p)
	}
}

func csvRoles(r *csv.Reader, idx headerIndex) (*Document, error) {
	d := &Document{HasRoles: true}
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			return d, nil
		}
		if err != nil {
			return nil, apperr.ErrInvalidArgument.With("csv", "unreadable").
				With("line", strconv.Itoa(line)).Wrap(err)
		}
		if blank(rec) {
			continue
		}
		if len(d.Roles) >= maxRows {
			return nil, apperr.ErrInvalidArgument.With("csv", "too many rows").
				With("max", strconv.Itoa(maxRows))
		}
		name := idx.get(rec, "role")
		if name == "" {
			name = idx.get(rec, "name")
		}
		d.Roles = append(d.Roles, Role{
			Name:              name,
			Description:       idx.get(rec, "description"),
			Permissions:       multi(idx.get(rec, "permissions")),
			Patterns:          multi(idx.get(rec, "patterns")),
			AllowedRealmKinds: multi(idx.get(rec, "allowed_realm_kinds")),
		})
	}
}

// headerIndex maps a column name to its position. Matching strips every
// separator, so "Min Assurance", "min-assurance" and "MIN_ASSURANCE" are one
// column — the same rule the workbook importer uses, because the operator
// exporting the file did not read either schema.
type headerIndex map[string]int

func index(header []string) headerIndex {
	idx := make(headerIndex, len(header))
	for i, h := range header {
		k := normalise(h)
		if k == "" {
			continue
		}
		if _, dup := idx[k]; !dup {
			idx[k] = i
		}
	}
	return idx
}

func normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (h headerIndex) has(name string) bool {
	_, ok := h[normalise(name)]
	return ok
}

func (h headerIndex) get(rec []string, name string) string {
	i, ok := h[normalise(name)]
	if !ok || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

// multi splits a cell holding several values. Semicolons and commas both
// work: a comma only reaches here from inside a quoted cell, where it was
// never a delimiter, and telling an operator their quoted list was the wrong
// punctuation helps nobody.
func multi(cell string) []string {
	if cell == "" {
		return nil
	}
	parts := strings.FieldsFunc(cell, func(r rune) bool { return r == ';' || r == ',' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func blank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
