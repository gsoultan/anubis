package provisioningdomain

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gsoultan/anubis/internal/provisioning/domain/row"
	"github.com/gsoultan/anubis/internal/provisioning/domain/schema"
)

// MaxRowsPerSheet bounds one sheet of an uploaded workbook. The limit is
// enforced here as well as at the transport because Parse builds maps
// keyed by values out of the file; a sheet past the limit is refused
// whole rather than truncated, since a silently half-imported directory
// is worse than one that plainly failed.
const MaxRowsPerSheet = 20000

// Parse turns raw sheet tables into typed rows.
//
// It is deliberately total: a row it cannot use becomes a RowIssue and the
// rest of the workbook still parses. Refusing an entire file over one bad
// cell would send an operator round a fix-one-thing-and-retry loop for
// every mistake in a four thousand row spreadsheet — they should get the
// whole list at once.
func Parse(tables map[string]Table) (Workbook, []RowIssue) {
	byName := make(map[string]Table, len(tables))
	for name, t := range tables {
		byName[strings.ToLower(strings.TrimSpace(name))] = t
	}

	var wb Workbook
	var issues []RowIssue
	for _, spec := range schema.Workbook() {
		t, ok := byName[strings.ToLower(spec.Name)]
		if !ok {
			// Not every import carries all three sheets. A workbook of
			// people alone is a normal thing to upload.
			continue
		}
		idx, missing := schema.Index(spec, t.Header)
		if len(missing) > 0 {
			issues = append(issues, RowIssue{
				Sheet:   spec.Name,
				Message: "missing required column(s): " + strings.Join(missing, ", "),
			})
			continue
		}
		if len(t.Rows) > MaxRowsPerSheet {
			issues = append(issues, RowIssue{
				Sheet: spec.Name,
				Message: fmt.Sprintf("%d rows exceeds the %d row limit — split the import into smaller files",
					len(t.Rows), MaxRowsPerSheet),
			})
			continue
		}

		switch spec.Name {
		case schema.SheetPeople:
			wb.People, issues = parsePeople(spec, idx, t, issues)
		case schema.SheetGrants:
			wb.Grants, issues = parseGrants(spec, idx, t, issues)
		case schema.SheetMemberships:
			wb.Memberships, issues = parseMemberships(spec, idx, t, issues)
		}
	}
	return wb, issues
}

func parsePeople(spec schema.SheetSpec, idx schema.HeaderIndex, t Table, issues []RowIssue) ([]row.Person, []RowIssue) {
	out := make([]row.Person, 0, len(t.Rows))
	for i, cells := range t.Rows {
		line := i + 2
		if idx.Empty(cells) {
			continue
		}
		p := row.Person{
			Row:         line,
			Realm:       idx.Value(cells, schema.ColRealm),
			Username:    idx.Value(cells, schema.ColUsername),
			Email:       idx.Value(cells, schema.ColEmail),
			Category:    idx.Value(cells, schema.ColCategory),
			ExternalRef: idx.Value(cells, schema.ColExternalRef),
		}
		bad := false
		for _, c := range []struct{ key, val string }{
			{schema.ColRealm, p.Realm}, {schema.ColUsername, p.Username},
		} {
			if c.val == "" {
				issues = append(issues, required(spec.Name, line, c.key))
				bad = true
			}
		}
		if raw := idx.Value(cells, schema.ColAssuranceLevel); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 4 {
				issues = append(issues, RowIssue{Sheet: spec.Name, Row: line,
					Column: schema.ColAssuranceLevel, Message: "must be a whole number from 1 to 4"})
				bad = true
			}
			p.AssuranceLevel = n
		}
		if bad {
			continue
		}
		out = append(out, p)
	}
	return out, issues
}

func parseGrants(spec schema.SheetSpec, idx schema.HeaderIndex, t Table, issues []RowIssue) ([]row.Grant, []RowIssue) {
	// Rows naming the same person, role and expiry fold into one grant.
	// All three maps are keyed by values out of the file and so are
	// bounded by MaxRowsPerSheet, checked before Parse gets here.
	order := make([]string, 0, len(t.Rows))
	byKey := make(map[string]*row.Grant, len(t.Rows))
	sawScoped := make(map[string]bool, len(t.Rows))
	sawUnscoped := make(map[string]bool, len(t.Rows))

	for i, cells := range t.Rows {
		line := i + 2
		if idx.Empty(cells) {
			continue
		}
		g := row.Grant{
			Realm:    idx.Value(cells, schema.ColRealm),
			Username: idx.Value(cells, schema.ColUsername),
			Role:     idx.Value(cells, schema.ColRole),
			Reason:   idx.Value(cells, schema.ColReason),
		}
		bad := false
		for _, c := range []struct{ key, val string }{
			{schema.ColRealm, g.Realm}, {schema.ColUsername, g.Username}, {schema.ColRole, g.Role},
		} {
			if c.val == "" {
				issues = append(issues, required(spec.Name, line, c.key))
				bad = true
			}
		}

		axis := idx.Value(cells, schema.ColScopeAxis)
		ref := idx.Value(cells, schema.ColScopeRef)
		switch {
		case axis != "" && ref == "":
			issues = append(issues, RowIssue{Sheet: spec.Name, Row: line, Column: schema.ColScopeRef,
				Message: "required when " + schema.ColScopeAxis + " is filled in"})
			bad = true
		case axis == "" && ref != "":
			issues = append(issues, RowIssue{Sheet: spec.Name, Row: line, Column: schema.ColScopeAxis,
				Message: "required when " + schema.ColScopeRef + " is filled in"})
			bad = true
		}

		inherit := true
		if raw := idx.Value(cells, schema.ColScopeInherit); raw != "" {
			v, err := parseBool(raw)
			if err != nil {
				issues = append(issues, RowIssue{Sheet: spec.Name, Row: line,
					Column: schema.ColScopeInherit, Message: "must be true or false"})
				bad = true
			}
			inherit = v
		}

		validUntilRaw := idx.Value(cells, schema.ColValidUntil)
		if validUntilRaw != "" {
			until, err := parseDate(validUntilRaw)
			if err != nil {
				issues = append(issues, RowIssue{Sheet: spec.Name, Row: line,
					Column: schema.ColValidUntil, Message: "must be a date as YYYY-MM-DD"})
				bad = true
			} else {
				g.ValidUntil = &until
			}
		}
		if bad {
			continue
		}

		key := strings.Join([]string{g.Realm, g.Username, g.Role, validUntilRaw, g.Reason}, "\x00")
		if axis == "" {
			sawUnscoped[key] = true
		} else {
			sawScoped[key] = true
		}
		existing, ok := byKey[key]
		if !ok {
			g.Rows = []int{line}
			byKey[key] = &g
			order = append(order, key)
			existing = &g
		} else {
			existing.Rows = append(existing.Rows, line)
		}
		if axis != "" && !hasScope(existing.Scopes, axis, ref) {
			existing.Scopes = append(existing.Scopes, row.GrantScope{Axis: axis, Ref: ref, Inherit: inherit})
		}
	}

	out := make([]row.Grant, 0, len(order))
	for _, k := range order {
		g := byKey[k]
		// One role granted to one person both with and without a scope is
		// ambiguous, and the two readings differ by exactly how much
		// access the person ends up with. Guessing either way is worse
		// than saying so: fail closed and let the operator decide.
		if sawScoped[k] && sawUnscoped[k] {
			issues = append(issues, RowIssue{Sheet: spec.Name, Row: g.Line(), Column: schema.ColScopeAxis,
				Message: fmt.Sprintf("rows %s grant %q to the same person both with and without a scope — "+
					"make them all scoped or all unscoped", joinRows(g.Rows), g.Role)})
			continue
		}
		out = append(out, *g)
	}
	return out, issues
}

func hasScope(scopes []row.GrantScope, axis, ref string) bool {
	for _, s := range scopes {
		if s.Axis == axis && s.Ref == ref {
			return true
		}
	}
	return false
}

func joinRows(rows []int) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = strconv.Itoa(r)
	}
	return strings.Join(parts, ", ")
}

func parseMemberships(spec schema.SheetSpec, idx schema.HeaderIndex, t Table, issues []RowIssue) ([]row.Membership, []RowIssue) {
	out := make([]row.Membership, 0, len(t.Rows))
	for i, cells := range t.Rows {
		line := i + 2
		if idx.Empty(cells) {
			continue
		}
		m := row.Membership{
			Row:      line,
			Realm:    idx.Value(cells, schema.ColRealm),
			Username: idx.Value(cells, schema.ColUsername),
			Name:     idx.Value(cells, schema.ColMembership),
		}
		bad := false
		for _, c := range []struct{ key, val string }{
			{schema.ColRealm, m.Realm}, {schema.ColUsername, m.Username}, {schema.ColMembership, m.Name},
		} {
			if c.val == "" {
				issues = append(issues, required(spec.Name, line, c.key))
				bad = true
			}
		}
		if bad {
			continue
		}
		out = append(out, m)
	}
	return out, issues
}

func required(sheet string, line int, column string) RowIssue {
	return RowIssue{Sheet: sheet, Row: line, Column: column, Message: "required"}
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1":
		return true, nil
	case "false", "no", "n", "0":
		return false, nil
	}
	return false, fmt.Errorf("not a true/false value: %q", s)
}

// parseDate accepts only unambiguous dates. The spreadsheet reader has
// already turned real Excel date cells into ISO, and accepting 01/02/2027
// would mean guessing between 1 February and 2 January — a grant that
// expires eleven months early is not a guess worth making.
func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("not an ISO date: %q", s)
}
