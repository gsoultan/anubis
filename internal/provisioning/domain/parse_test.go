package provisioningdomain

import (
	"testing"
)

func peopleTable(rows ...[]string) map[string]Table {
	return map[string]Table{
		"People": {Header: []string{"realm", "username", "email", "category", "external_ref", "assurance_level"}, Rows: rows},
	}
}

func grantTable(rows ...[]string) map[string]Table {
	return map[string]Table{
		"Grants": {Header: []string{"realm", "username", "role", "scope_axis", "scope_ref", "scope_inherit", "valid_until", "reason"}, Rows: rows},
	}
}

func TestParsePeople(t *testing.T) {
	wb, issues := Parse(peopleTable(
		[]string{"staff", "ada", "ada@example.com", "employee", "HR-1", "2"},
		[]string{"staff", "grace"},
	))
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(wb.People) != 2 {
		t.Fatalf("got %d people, want 2", len(wb.People))
	}
	p := wb.People[0]
	if p.Row != 2 || p.Realm != "staff" || p.Username != "ada" || p.Email != "ada@example.com" ||
		p.Category != "employee" || p.ExternalRef != "HR-1" || p.AssuranceLevel != 2 {
		t.Fatalf("person = %+v", p)
	}
	// A short row is not an error; the columns simply are not there.
	if wb.People[1].Row != 3 || wb.People[1].Email != "" {
		t.Fatalf("short row = %+v", wb.People[1])
	}
}

// The row number in an issue is the number the operator sees in Excel, so
// they can go straight to the cell. Off by one here and every error
// message points at the wrong line.
func TestIssueRowNumbersAreSpreadsheetRows(t *testing.T) {
	_, issues := Parse(peopleTable(
		[]string{"staff", "ada"},
		[]string{"", "grace"},
	))
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
	}
	if issues[0].Row != 3 {
		t.Errorf("issue row = %d, want 3 (header is row 1)", issues[0].Row)
	}
	if issues[0].Column != "realm" || issues[0].Sheet != "People" {
		t.Errorf("issue = %+v", issues[0])
	}
}

func TestParseSkipsBlankRows(t *testing.T) {
	wb, issues := Parse(peopleTable(
		[]string{"staff", "ada"},
		nil,
		[]string{"", "", "", "", "", ""},
		[]string{"staff", "grace"},
	))
	if len(issues) != 0 {
		t.Fatalf("blank rows should not be errors: %+v", issues)
	}
	if len(wb.People) != 2 {
		t.Fatalf("got %d people, want 2", len(wb.People))
	}
	if wb.People[1].Row != 5 {
		t.Errorf("row number = %d, want 5", wb.People[1].Row)
	}
}

// Operators reorder columns and add working columns of their own. Reading
// by header rather than position is what keeps that from silently
// importing an email address into the category field.
func TestParseReadsByHeaderNotPosition(t *testing.T) {
	wb, issues := Parse(map[string]Table{
		"people": {
			Header: []string{"Notes", "User Name", "REALM", "email"},
			Rows:   [][]string{{"ignore me", "ada", "staff", "ada@example.com"}},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	p := wb.People[0]
	if p.Username != "ada" || p.Realm != "staff" || p.Email != "ada@example.com" {
		t.Fatalf("person = %+v", p)
	}
}

func TestMissingRequiredColumnIsReportedOncePerSheet(t *testing.T) {
	_, issues := Parse(map[string]Table{
		"People": {Header: []string{"realm", "email"}, Rows: [][]string{{"staff", "a@b.c"}, {"staff", "d@e.f"}}},
	})
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
	}
	if issues[0].Row != 0 {
		t.Errorf("a sheet-level issue should not name a row, got %d", issues[0].Row)
	}
}

func TestBadAssuranceLevelIsRejected(t *testing.T) {
	for _, bad := range []string{"0", "5", "high", "-1"} {
		_, issues := Parse(peopleTable([]string{"staff", "ada", "", "", "", bad}))
		if len(issues) != 1 {
			t.Errorf("assurance_level %q: got %d issues, want 1", bad, len(issues))
		}
	}
}

func TestParseGrantsWithScope(t *testing.T) {
	wb, issues := Parse(grantTable(
		[]string{"staff", "ada", "support", "region", "apac", "false", "2027-01-31", "onboarding"},
	))
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	g := wb.Grants[0]
	if g.Line() != 2 || g.Role != "support" || g.Reason != "onboarding" {
		t.Fatalf("grant = %+v", g)
	}
	if len(g.Scopes) != 1 || g.Scopes[0].Axis != "region" || g.Scopes[0].Ref != "apac" || g.Scopes[0].Inherit {
		t.Fatalf("scopes = %+v", g.Scopes)
	}
	if g.ValidUntil == nil || g.ValidUntil.Format("2006-01-02") != "2027-01-31" {
		t.Fatalf("valid_until = %v", g.ValidUntil)
	}
}

// Scope inherit defaults to true: a grant on a branch normally means the
// branch and everything under it.
func TestScopeInheritDefaultsTrue(t *testing.T) {
	wb, _ := Parse(grantTable([]string{"staff", "ada", "support", "region", "apac"}))
	if !wb.Grants[0].Scopes[0].Inherit {
		t.Error("inherit should default to true")
	}
}

// Two regions on two rows is one grant over both, not two grants: scopes
// within an axis are OR-ed, and one grant is one thing to revoke later.
func TestGrantRowsWithSameRoleFoldIntoOneGrant(t *testing.T) {
	wb, issues := Parse(grantTable(
		[]string{"staff", "ada", "support", "region", "apac", "true", "", ""},
		[]string{"staff", "ada", "support", "region", "emea", "true", "", ""},
		[]string{"staff", "ada", "auditor", "region", "apac", "true", "", ""},
	))
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(wb.Grants) != 2 {
		t.Fatalf("got %d grants, want 2: %+v", len(wb.Grants), wb.Grants)
	}
	if len(wb.Grants[0].Scopes) != 2 {
		t.Fatalf("first grant scopes = %+v", wb.Grants[0].Scopes)
	}
	if got := wb.Grants[0].Rows; len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("contributing rows = %v, want [2 3]", got)
	}
}

// A different expiry is a different grant even for the same role.
func TestGrantsWithDifferentExpiryDoNotFold(t *testing.T) {
	wb, _ := Parse(grantTable(
		[]string{"staff", "ada", "support", "region", "apac", "true", "2027-01-31", ""},
		[]string{"staff", "ada", "support", "region", "emea", "true", "2028-01-31", ""},
	))
	if len(wb.Grants) != 2 {
		t.Fatalf("got %d grants, want 2", len(wb.Grants))
	}
}

// Mixing a scoped and an unscoped row for the same role is ambiguous, and
// the two readings differ by exactly how much access the person gets.
// Guessing is worse than refusing.
func TestScopedAndUnscopedRowsForSameRoleAreRefused(t *testing.T) {
	wb, issues := Parse(grantTable(
		[]string{"staff", "ada", "support", "region", "apac", "true", "", ""},
		[]string{"staff", "ada", "support", "", "", "", "", ""},
	))
	if len(wb.Grants) != 0 {
		t.Fatalf("an ambiguous grant must not be imported, got %+v", wb.Grants)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
	}
}

func TestScopeAxisAndRefMustComeTogether(t *testing.T) {
	_, issues := Parse(grantTable([]string{"staff", "ada", "support", "region", "", "", "", ""}))
	if len(issues) != 1 || issues[0].Column != "scope_ref" {
		t.Fatalf("issues = %+v", issues)
	}
	_, issues = Parse(grantTable([]string{"staff", "ada", "support", "", "apac", "", "", ""}))
	if len(issues) != 1 || issues[0].Column != "scope_axis" {
		t.Fatalf("issues = %+v", issues)
	}
}

// An ambiguous date would silently expire a grant eleven months early.
func TestAmbiguousDatesAreRejected(t *testing.T) {
	for _, bad := range []string{"01/02/2027", "31-01-2027", "Jan 31 2027", "2027-13-01"} {
		_, issues := Parse(grantTable([]string{"staff", "ada", "support", "", "", "", bad, ""}))
		if len(issues) != 1 {
			t.Errorf("valid_until %q: got %d issues, want 1", bad, len(issues))
		}
	}
	for _, good := range []string{"2027-01-31", "2027-01-31T00:00:00Z"} {
		_, issues := Parse(grantTable([]string{"staff", "ada", "support", "", "", "", good, ""}))
		if len(issues) != 0 {
			t.Errorf("valid_until %q should parse: %+v", good, issues)
		}
	}
}

func TestParseMemberships(t *testing.T) {
	wb, issues := Parse(map[string]Table{
		"Memberships": {Header: []string{"realm", "username", "membership"},
			Rows: [][]string{{"staff", "ada", "support-team"}, {"staff", "", "support-team"}}},
	})
	if len(wb.Memberships) != 1 || wb.Memberships[0].Name != "support-team" {
		t.Fatalf("memberships = %+v", wb.Memberships)
	}
	if len(issues) != 1 || issues[0].Row != 3 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestUnknownSheetsAreIgnored(t *testing.T) {
	wb, issues := Parse(map[string]Table{
		"Instructions": {Header: []string{"how to use this"}, Rows: [][]string{{"fill in the People sheet"}}},
		"People":       {Header: []string{"realm", "username"}, Rows: [][]string{{"staff", "ada"}}},
	})
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(wb.People) != 1 {
		t.Fatalf("people = %+v", wb.People)
	}
}

func TestReportBoundsIssues(t *testing.T) {
	var r ImportReport
	for i := 0; i < MaxReportIssues+25; i++ {
		r.AddIssue(RowIssue{Sheet: "People", Row: i, Message: "bad"})
	}
	if len(r.Issues) != MaxReportIssues {
		t.Errorf("kept %d issues, want %d", len(r.Issues), MaxReportIssues)
	}
	if r.IssuesOmitted != 25 {
		t.Errorf("omitted = %d, want 25", r.IssuesOmitted)
	}
	// A truncated report must never read as a clean one.
	if r.OK() {
		t.Error("a report with omitted issues is not OK")
	}
}
