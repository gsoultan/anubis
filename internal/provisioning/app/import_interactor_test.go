package provisioningapp

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/xlsx"
	"github.com/gsoultan/anubis/internal/provisioning/domain/schema"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

type harness struct {
	uc     ImportUsecase
	dir    *fakeDirectory
	people *fakePeople
	access *fakeAccess
	writer *fakeAccessWriter
	scope  *fakeScope
	tx     *fakeTx
	audit  *fakeAudit
}

// newHarness builds the interactor for an operator holding the given roles
// on tenant-1. No roles means no assignment: somebody who can sign in and
// administer nothing.
func newHarness(roles ...controldomain.OperatorRole) *harness {
	assignments := make([]controldomain.AssignmentRecord, 0, len(roles))
	for _, r := range roles {
		assignments = append(assignments, controldomain.AssignmentRecord{
			OperatorID: "op-1", TenantID: "tenant-1", Role: r,
		})
	}
	h := &harness{
		dir: &fakeDirectory{
			realms: map[string]string{"staff": "realm-staff"},
			people: map[string]string{},
		},
		people: &fakePeople{},
		access: &fakeAccess{
			roles:       map[string]string{"support": "role-support"},
			memberships: []membership.MembershipRecord{{ID: "mem-1", Name: "support-team"}},
			grants:      map[string][]grant.GrantRecord{},
		},
		writer: &fakeAccessWriter{},
		scope:  &fakeScope{nodes: map[string]string{"region\x00apac": "node-apac", "region\x00emea": "node-emea"}},
		tx:     &fakeTx{},
		audit:  &fakeAudit{},
	}
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	h.uc = NewImportInteractor(fakeOperators{rows: assignments}, func() time.Time { return now },
		h.dir, h.people, h.access, h.writer, h.scope,
		fixedClock{t: now}, h.tx, h.audit)
	return h
}

func adminCtx() context.Context {
	// A platform operator working in tenant-1 — the only population that can
	// administer anything (ADR-0011, revised).
	return authctx.With(context.Background(),
		&authctx.Principal{IdentityID: "op-1", TenantID: "tenant-1", Platform: true})
}

// book builds an .xlsx upload from sheet name to header and rows.
func book(t *testing.T, sheets ...xlsx.Sheet) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := xlsx.Write(&buf, sheets); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sheet(name string, header []string, rows ...[]string) xlsx.Sheet {
	cols := make([]xlsx.Column, len(header))
	for i, h := range header {
		cols[i] = xlsx.Column{Header: h}
	}
	return xlsx.Sheet{Name: name, Columns: cols, Rows: rows}
}

func peopleSheet(rows ...[]string) xlsx.Sheet {
	return sheet(schema.SheetPeople, []string{"realm", "username", "email"}, rows...)
}

func grantsSheet(rows ...[]string) xlsx.Sheet {
	return sheet(schema.SheetGrants,
		[]string{"realm", "username", "role", "scope_axis", "scope_ref", "scope_inherit", "valid_until", "reason"}, rows...)
}

func membershipsSheet(rows ...[]string) xlsx.Sheet {
	return sheet(schema.SheetMemberships, []string{"realm", "username", "membership"}, rows...)
}

// --- template ---------------------------------------------------------

// The template has to survive a round trip through the reader that will
// parse it back, and its headers have to be the ones the parser looks for.
// If these drift, every import silently reports missing columns.
func TestTemplateMatchesTheSchemaThatParsesIt(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	data, err := h.uc.ImportTemplate(adminCtx())
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := xlsx.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("the generated template is not readable: %v", err)
	}
	byName := map[string]xlsx.Sheet{}
	for _, s := range sheets {
		byName[s.Name] = s
	}
	for _, spec := range schema.Workbook() {
		got, ok := byName[spec.Name]
		if !ok {
			t.Fatalf("template has no %s sheet", spec.Name)
		}
		idx, missing := schema.Index(spec, got.Header())
		if len(missing) > 0 {
			t.Errorf("%s: template is missing %v", spec.Name, missing)
		}
		for _, c := range spec.Columns {
			if !idx.Has(c.Key) {
				t.Errorf("%s: template has no %s column", spec.Name, c.Key)
			}
		}
	}
	if _, ok := byName[schema.SheetInstructions]; !ok {
		t.Error("template has no Instructions sheet")
	}
}

// A sample row left in by an operator who did not notice it would be a
// fictional person created in a real directory.
func TestTemplateDataSheetsShipEmpty(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	data, _ := h.uc.ImportTemplate(adminCtx())
	sheets, _ := xlsx.Read(bytes.NewReader(data), int64(len(data)))
	for _, s := range sheets {
		if s.Name == schema.SheetInstructions {
			continue
		}
		if len(s.Rows) != 0 {
			t.Errorf("%s ships %d data rows, want 0: %v", s.Name, len(s.Rows), s.Rows)
		}
	}
}

func TestTemplateRequiresPermission(t *testing.T) {
	h := newHarness()
	if _, err := h.uc.ImportTemplate(adminCtx()); err == nil {
		t.Fatal("want permission denied")
	}
}

func TestTemplateRequiresAuthentication(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	if _, err := h.uc.ImportTemplate(context.Background()); err == nil {
		t.Fatal("want unauthenticated")
	}
}

// --- dry run ----------------------------------------------------------

// A dry run is the intended first step, so it must not write — not even
// a rolled-back write that takes locks and burns ids.
func TestDryRunWritesNothing(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Dry:  true,
		Data: book(t, peopleSheet([]string{"staff", "ada", "ada@example.com"}), grantsSheet([]string{"staff", "ada", "support", "", "", "", "", ""})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Dry || rep.Applied {
		t.Errorf("dry=%v applied=%v", rep.Dry, rep.Applied)
	}
	if rep.PeopleCreated != 1 || rep.GrantsCreated != 1 {
		t.Errorf("projection = %+v", rep)
	}
	if h.tx.entered != 0 {
		t.Error("a dry run opened a transaction")
	}
	if len(h.people.created) != 0 || len(h.writer.grants) != 0 {
		t.Error("a dry run wrote something")
	}
	if len(h.audit.events) != 0 {
		t.Error("a dry run emitted an audit event")
	}
}

// --- apply ------------------------------------------------------------

func TestImportCreatesPeopleGrantsAndMemberships(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t,
			peopleSheet([]string{"staff", "ada", "ada@example.com"}),
			grantsSheet([]string{"staff", "ada", "support", "region", "apac", "true", "2027-01-31", "onboarding"}),
			membershipsSheet([]string{"staff", "ada", "support-team"}),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Applied || !rep.OK() {
		t.Fatalf("report = %+v", rep)
	}
	if len(h.people.created) != 1 || h.people.created[0].Username != "ada" {
		t.Fatalf("created = %+v", h.people.created)
	}
	if len(h.writer.grants) != 1 {
		t.Fatalf("grants = %+v", h.writer.grants)
	}
	g := h.writer.grants[0]
	if g.IdentityID != "new-ada" || g.RoleID != "role-support" || g.Reason != "onboarding" {
		t.Errorf("grant = %+v", g)
	}
	if len(g.Scopes) != 1 || g.Scopes[0].NodeID != "node-apac" || !g.Scopes[0].Inherit {
		t.Errorf("grant scopes = %+v", g.Scopes)
	}
	if g.ValidUntil == nil || g.ValidUntil.Format("2006-01-02") != "2027-01-31" {
		t.Errorf("valid until = %v", g.ValidUntil)
	}
	if len(h.writer.assigned) != 1 || h.writer.assigned[0] != [2]string{"mem-1", "new-ada"} {
		t.Errorf("assigned = %+v", h.writer.assigned)
	}
	if len(h.audit.events) != 1 || h.audit.events[0].Action != "provisioning.import" {
		t.Errorf("audit = %+v", h.audit.events)
	}
}

// The whole point of importing people and their access together is that
// the Grants sheet can name somebody the People sheet is creating.
func TestGrantsCanNamePeopleTheSameImportCreates(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t,
			peopleSheet([]string{"staff", "grace", ""}),
			grantsSheet([]string{"staff", "grace", "support", "", "", "", "", ""}),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("issues: %+v", rep.Issues)
	}
	if len(h.writer.grants) != 1 || h.writer.grants[0].IdentityID != "new-grace" {
		t.Fatalf("grants = %+v", h.writer.grants)
	}
}

// Re-running the same file is a normal thing to do and must not pile up
// duplicate people or duplicate grants.
func TestReRunningAnImportIsANoOp(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	h.dir.people["realm-staff\x00ada"] = "id-ada"
	h.access.grants["id-ada"] = []grant.GrantRecord{{RoleID: "role-support"}}

	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t,
			peopleSheet([]string{"staff", "ada", ""}),
			grantsSheet([]string{"staff", "ada", "support", "", "", "", "", ""}),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.PeopleExisting != 1 || rep.PeopleCreated != 0 {
		t.Errorf("people: existing=%d created=%d", rep.PeopleExisting, rep.PeopleCreated)
	}
	if rep.GrantsSkipped != 1 || rep.GrantsCreated != 0 {
		t.Errorf("grants: skipped=%d created=%d", rep.GrantsSkipped, rep.GrantsCreated)
	}
	if len(h.people.created) != 0 || len(h.writer.grants) != 0 {
		t.Error("a re-run wrote something")
	}
}

// An expired grant is not a live one, so the role is granted again.
func TestExpiredGrantIsNotTreatedAsHeld(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	h.dir.people["realm-staff\x00ada"] = "id-ada"
	past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	h.access.grants["id-ada"] = []grant.GrantRecord{{RoleID: "role-support", ValidUntil: &past}}

	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t, grantsSheet([]string{"staff", "ada", "support", "", "", "", "", ""})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.GrantsCreated != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

// One role granted to four thousand people should not ask the database
// the same question four thousand times.
func TestGrantLookupsAreCachedPerPerson(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	h.dir.people["realm-staff\x00ada"] = "id-ada"
	if _, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Dry: true,
		Data: book(t, grantsSheet(
			[]string{"staff", "ada", "support", "region", "apac", "true", "", ""},
			[]string{"staff", "ada", "support", "region", "emea", "true", "", ""},
		)),
	}); err != nil {
		t.Fatal(err)
	}
	if h.access.listCalls != 1 {
		t.Errorf("ListGrants called %d times, want 1", h.access.listCalls)
	}
}

// --- refusal ----------------------------------------------------------

// An import applies whole or not at all: one unresolvable row must not
// leave a half-imported directory behind.
func TestAnyIssueBlocksTheWholeImport(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t,
			peopleSheet([]string{"staff", "ada", ""}, []string{"nosuchrealm", "grace", ""}),
			grantsSheet([]string{"staff", "ada", "support", "", "", "", "", ""}),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Applied {
		t.Fatal("an import with issues must not apply")
	}
	if len(rep.Issues) != 1 || rep.Issues[0].Row != 3 {
		t.Fatalf("issues = %+v", rep.Issues)
	}
	if h.tx.entered != 0 || len(h.people.created) != 0 || len(h.writer.grants) != 0 {
		t.Error("a refused import wrote something")
	}
}

func TestUnresolvableNamesBecomeIssues(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	h.dir.people["realm-staff\x00ada"] = "id-ada"
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Dry: true,
		Data: book(t,
			grantsSheet(
				[]string{"staff", "ada", "nosuchrole", "", "", "", "", ""},
				[]string{"staff", "ada", "support", "region", "nosuchnode", "true", "", ""},
				[]string{"staff", "nosuchperson", "support", "", "", "", "", ""},
			),
			membershipsSheet([]string{"staff", "ada", "nosuchmembership"}),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Issues) != 4 {
		t.Fatalf("got %d issues, want 4: %+v", len(rep.Issues), rep.Issues)
	}
	cols := map[string]bool{}
	for _, i := range rep.Issues {
		cols[i.Column] = true
	}
	for _, want := range []string{schema.ColRole, schema.ColScopeRef, schema.ColUsername, schema.ColMembership} {
		if !cols[want] {
			t.Errorf("no issue reported against %s: %+v", want, rep.Issues)
		}
	}
}

// --- permissions ------------------------------------------------------

// Someone who may create people but not grant roles can still import a
// People-only workbook.
func TestPeopleOnlyImportDoesNotNeedAccessPermission(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t, peopleSheet([]string{"staff", "ada", ""})),
	})
	if err != nil {
		t.Fatalf("a People-only import should not need grant admin: %v", err)
	}
	if !rep.Applied || rep.PeopleCreated != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestGrantsRequireAccessPermission(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	_, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t, grantsSheet([]string{"staff", "ada", "support", "", "", "", "", ""})),
	})
	if err == nil {
		t.Fatal("want permission denied")
	}
	if apperr.AsError(err).Code != apperr.ErrPermissionDenied.Code {
		t.Fatalf("err = %v", err)
	}
}

func TestMembershipsRequireAccessPermission(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	if _, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t, membershipsSheet([]string{"staff", "ada", "support-team"})),
	}); err == nil {
		t.Fatal("want permission denied")
	}
}

// A dry run resolves real role and scope names, so it answers questions
// about the directory that a caller without these rights may not ask.
func TestDryRunIsGatedTheSameAsApply(t *testing.T) {
	h := newHarness(controldomain.RoleSupport)
	if _, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Dry:  true,
		Data: book(t, grantsSheet([]string{"staff", "ada", "support", "", "", "", "", ""})),
	}); err == nil {
		t.Fatal("a dry run must be gated like an apply")
	}
}

func TestImportRequiresBaselinePermission(t *testing.T) {
	h := newHarness()
	if _, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t, peopleSheet([]string{"staff", "ada", ""})),
	}); err == nil {
		t.Fatal("want permission denied")
	}
}

// --- input handling ---------------------------------------------------

func TestRejectsJunkAndOversizedUploads(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	cases := map[string][]byte{
		"empty":     nil,
		"not xlsx":  []byte("realm,username\nstaff,ada\n"),
		"too large": bytes.Repeat([]byte("x"), MaxUploadBytes+1),
	}
	for name, data := range cases {
		_, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{Data: data})
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		if apperr.AsError(err).Code != apperr.ErrInvalidArgument.Code {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestWorkbookWithNoKnownSheetsIsHarmless(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	rep, err := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Data: book(t, sheet("Sheet1", []string{"a", "b"}, []string{"1", "2"})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed() != 0 || h.tx.entered != 0 {
		t.Errorf("report = %+v", rep)
	}
}

// The report names the sheet and the spreadsheet row, since that is what
// the operator has in front of them.
func TestIssuesNameTheSheetAndRow(t *testing.T) {
	h := newHarness(controldomain.RoleAdmin)
	rep, _ := h.uc.ImportWorkbook(adminCtx(), ImportInput{
		Dry:  true,
		Data: book(t, peopleSheet([]string{"", "ada", ""})),
	})
	if len(rep.Issues) != 1 {
		t.Fatalf("issues = %+v", rep.Issues)
	}
	i := rep.Issues[0]
	if i.Sheet != schema.SheetPeople || i.Row != 2 || i.Column != schema.ColRealm {
		t.Fatalf("issue = %+v", i)
	}
	if !strings.Contains(strings.ToLower(i.Message), "required") {
		t.Errorf("message = %q", i.Message)
	}
}
