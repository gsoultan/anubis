package convert

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/gsoultan/anubis/cmd/osiris2anubis/osiris"
	authzcatalog "github.com/gsoultan/anubis/internal/authz/app/catalog"
	"github.com/gsoultan/anubis/internal/platform/xlsx"
)

// A small Osiris: a training app with two policies, an HR "sees everything"
// policy, and three kinds of people. Every id is invented.
func fixture() (osiris.Catalog, osiris.People) {
	c := osiris.Catalog{
		Rules: []osiris.Rule{
			{ID: "r-list", Name: "LIST_TRAINING", Description: "See the training list"},
			{ID: "r-add", Name: "ADD_NEW_TRAINING"},
			{ID: "r-share", Name: "SHARE"},
			{ID: "r-full", Name: "TRAINING_FULL_SCOPE"},
			{ID: "r-dup1", Name: "ADD_USER"},
			{ID: "r-dup2", Name: "ADD__USER"},
			{ID: "r-orphan", Name: "EXPORT_LEGACY_REPORT"},
		},
		Policies: []osiris.Policy{
			{ID: "p-admin", Name: "Training Admin", Description: "Runs training", FeatureID: "f-trn"},
			{ID: "p-view", Name: "Training Viewer", FeatureID: "f-trn"},
			{ID: "p-hr", Name: "HR Full", FeatureID: "f-hr"},
		},
		Authorities: []osiris.Authority{
			{PolicyID: "p-admin", RuleID: "r-list"}, {PolicyID: "p-admin", RuleID: "r-add"},
			{PolicyID: "p-admin", RuleID: "r-share"},
			{PolicyID: "p-view", RuleID: "r-list"},
			{PolicyID: "p-hr", RuleID: "r-full"}, {PolicyID: "p-hr", RuleID: "r-list"},
			{PolicyID: "p-hr", RuleID: "r-dup1"}, {PolicyID: "p-hr", RuleID: "r-dup2"},
		},
		Features: []osiris.Feature{{ID: "f-trn", Name: "Training", ModuleID: "m-ld"}, {ID: "f-hr", Name: "People", ModuleID: "m-hr"}},
		Modules:  []osiris.Module{{ID: "m-ld", Name: "Learning", AppID: "a-ess"}, {ID: "m-hr", Name: "HR", AppID: "a-ess"}},
		Apps:     []osiris.App{{ID: "a-ess", Name: "ESS"}},
		Groups: []osiris.Group{
			{ID: "g-trn", Code: "TRN_ADMIN", Name: "Training admins", PolicyIDs: []string{"p-admin"}},
			{ID: "g-hr", Code: "HRD", Name: "HR department", PolicyIDs: []string{"p-hr", "p-view"}},
			{ID: "g-app", Code: "APPLICANTS", Name: "Applicants", PolicyIDs: []string{"p-view"}},
		},
	}
	p := osiris.People{
		Users: []osiris.User{
			{ID: "u-dewi", Username: "Dewi.Pratama@impack.example", EntityType: "EMPLOYEE"},
			{ID: "u-rudi", Username: "rudi@impack.example", EntityType: "EMPLOYEE"},
			{ID: "u-ayu", Username: "Ayu.K@mail.example", EntityType: "APPLICANT"},
			{ID: "u-ayu2", Username: "ayu.k@mail.example", EntityType: "APPLICANT"},
			{ID: "u-kiosk", Username: "kiosk-1@impack.example", EntityType: "DEVICE"},
			{ID: "u-unknown", Username: "x@impack.example", EntityType: "SOMETHING_ELSE"},
		},
		Placements: []osiris.Placement{
			{ID: "s-dewi", UserID: "u-dewi", OrganizationID: "org-1", UnitID: "unit-7", GradeID: "grade-5", GroupIDs: []string{"g-trn"}},
			{ID: "s-rudi", UserID: "u-rudi", OrganizationID: "org-1", WorkplaceID: "wp-jkt", JobTypeID: "jt-1", GroupIDs: []string{"g-hr"}},
			{ID: "s-ayu", UserID: "u-ayu", GroupIDs: []string{"g-app"}},
			{ID: "s-kiosk", UserID: "u-kiosk", OrganizationID: "org-1"},
		},
	}
	return c, p
}

func TestRuleKeysAreDeterministicAndStoreable(t *testing.T) {
	for in, want := range map[string][2]string{
		"LIST_TRAINING":        {"training", "list"},
		"ADD_NEW_TRAINING":     {"new_training", "add"},
		"VIEW_EVALUATION_FORM": {"evaluation_form", "view"},
		"SHARE":                {"share", "use"},
		" list_training ":      {"training", "list"},
		"ADD-USER (OLD)":       {"add_user_old", "use"},
		"9LIVES_X":             {"x", "r_9lives"},
	} {
		r, a := RuleKey(in)
		if r != want[0] || a != want[1] {
			t.Errorf("%q → %s:%s, want %s:%s", in, r, a, want[0], want[1])
		}
	}
	for _, m := range []string{"FULL_DATA_SCOPE", "TRAINING_FULL_SCOPE", "training_full_scope"} {
		if !IsScopeMarker(m) {
			t.Errorf("%s is a scope marker", m)
		}
	}
	if IsScopeMarker("LIST_TRAINING") {
		t.Error("LIST_TRAINING is not a marker")
	}
	if Slug("Training  Admin / Lead") != "training-admin-lead" {
		t.Errorf("slug: %q", Slug("Training  Admin / Lead"))
	}
}

func TestTheCatalogBecomesAValidManifestAndARuleTable(t *testing.T) {
	c, _ := fixture()
	rep := NewReport()
	out := Catalog("osiris", c, rep)

	// What Anubis will accept.
	doc := authzcatalog.Document{Permissions: out.Manifest.Permissions, HasPermissions: true,
		Roles: out.Manifest.Roles, HasRoles: true}
	if err := doc.Validate(); err != nil {
		t.Fatalf("manifest does not validate: %v", err)
	}

	keys := map[string]authzcatalog.Permission{}
	for _, p := range out.Manifest.Permissions {
		keys[p.Resource+":"+p.Action] = p
	}
	if p, ok := keys["training:list"]; !ok || p.Description != "See the training list" || p.Module != "Learning" {
		t.Fatalf("training:list wrong: %+v (present=%v)", p, ok)
	}
	if _, ok := keys["training_full_scope:use"]; ok {
		t.Fatal("a scope marker became a permission")
	}
	// Two rules that clean to one key: the second keeps its whole name.
	if _, ok := keys["user:add"]; !ok {
		t.Fatal("ADD_USER missing")
	}
	if _, ok := keys["add__user:use"]; !ok {
		t.Fatalf("ADD__USER was merged into ADD_USER; keys %v", keysOf(keys))
	}
	if len(rep.KeyConflicts) != 1 {
		t.Fatalf("conflict not reported: %v", rep.KeyConflicts)
	}
	if !reflect.DeepEqual(rep.UnreferencedRules, []string{"EXPORT_LEGACY_REPORT"}) {
		t.Fatalf("orphan rules: %v", rep.UnreferencedRules)
	}

	roles := map[string]authzcatalog.Role{}
	for _, r := range out.Manifest.Roles {
		roles[r.Name] = r
	}
	if r := roles["training-admin"]; !reflect.DeepEqual(r.Permissions, []string{"new_training:add", "share:use", "training:list"}) || r.Description != "Runs training" {
		t.Fatalf("training-admin: %+v", r)
	}
	if r := roles["hr-full"]; !reflect.DeepEqual(r.Permissions, []string{"add__user:use", "training:list", "user:add"}) {
		t.Fatalf("hr-full carries the marker or lost a rule: %+v", r)
	}
	if out.RoleByPolicy["p-admin"] != "osiris.training-admin" {
		t.Fatalf("stored role name: %q", out.RoleByPolicy["p-admin"])
	}

	// rules.csv: every rule once, markers flagged, keys app-prefixed.
	table := map[string]RuleMapping{}
	for _, m := range out.Rules {
		table[m.Rule] = m
	}
	if len(table) != len(c.Rules) {
		t.Fatalf("rule table has %d of %d rules", len(table), len(c.Rules))
	}
	if m := table["LIST_TRAINING"]; m.Permission != "osiris:training:list" || m.Marker {
		t.Fatalf("LIST_TRAINING → %+v", m)
	}
	if m := table["TRAINING_FULL_SCOPE"]; !m.Marker || m.Permission != "" {
		t.Fatalf("marker → %+v", m)
	}
}

func TestPeopleAndGrantsFollowPlacementAndGroup(t *testing.T) {
	c, p := fixture()
	rep := NewReport()
	cat := Catalog("osiris", c, rep)
	out := People(DefaultOptions(), cat, c, p, rep)

	// People: realms by type, usernames lowercased and de-duplicated, the
	// unknown type skipped, the Osiris id carried as external_ref.
	usernames := map[string][]string{}
	for _, row := range out.People {
		usernames[row[0]] = append(usernames[row[0]], row[1])
		if row[4] == "" {
			t.Fatalf("no external_ref on %v", row)
		}
	}
	if !reflect.DeepEqual(usernames["internal"], []string{"dewi.pratama@impack.example", "kiosk-1@impack.example", "rudi@impack.example"}) {
		t.Fatalf("internal: %v", usernames["internal"])
	}
	if !reflect.DeepEqual(usernames["public"], []string{"ayu.k@mail.example"}) {
		t.Fatalf("public: %v", usernames["public"])
	}
	if len(rep.DuplicateUsernames) != 1 || rep.PeopleByRealm["(skipped: SOMETHING_ELSE)"] != 1 {
		t.Fatalf("report: dup %v skipped %v", rep.DuplicateUsernames, rep.PeopleByRealm)
	}

	// Grants: dewi's group at (org-1, unit-7) → one row per axis, grade
	// dropped; rudi's HR group confers a marker → everywhere, and his
	// viewer policy with it; ayu is unplaced → skipped by default.
	type g struct{ user, role, axis, ref string }
	var got []g
	for _, row := range out.Grants {
		got = append(got, g{row[1], row[2], row[3], row[4]})
	}
	want := []g{
		{"dewi.pratama@impack.example", "osiris.training-admin", "org", "org-1"},
		{"dewi.pratama@impack.example", "osiris.training-admin", "unit", "unit-7"},
		{"rudi@impack.example", "osiris.hr-full", "", ""},
		{"rudi@impack.example", "osiris.training-viewer", "", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("grants:\n got %+v\nwant %+v", got, want)
	}
	if rep.Unplaced != 1 || rep.UnplacedIncluded || rep.DroppedGrade != 1 || rep.DroppedJobType != 1 || rep.NoGroup != 1 {
		t.Fatalf("report: %+v", rep)
	}
	for _, row := range out.Grants {
		if row[3] != "" && (row[5] != "true" || row[6] != "false") {
			t.Fatalf("a placed grant must inherit and not exclude: %v", row)
		}
		if !strings.HasPrefix(row[8], "osiris group ") {
			t.Fatalf("reason must name the group: %v", row)
		}
	}

	// Opting in carries the unplaced applicant everywhere, and says so.
	o := DefaultOptions()
	o.UnplacedEverywhere = true
	rep2 := NewReport()
	out2 := People(o, Catalog("osiris", c, rep2), c, p, rep2)
	var ayu int
	for _, row := range out2.Grants {
		if row[1] == "ayu.k@mail.example" && row[3] == "" {
			ayu++
		}
	}
	if ayu != 1 || !strings.Contains(rep2.String(), "EVERYWHERE") {
		t.Fatalf("unplaced opt-in: rows %d report %q", ayu, rep2.String())
	}
}

// The workbook must be the one the importer reads: sheet names and column
// headers come from the import's own schema, and the rows round-trip.
func TestTheWorkbookRoundTripsThroughTheImportersReader(t *testing.T) {
	c, p := fixture()
	rep := NewReport()
	out := People(DefaultOptions(), Catalog("osiris", c, rep), c, p, rep)

	var buf bytes.Buffer
	if err := WriteWorkbook(&buf, out); err != nil {
		t.Fatal(err)
	}
	sheets, err := xlsx.Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]xlsx.Sheet{}
	for _, s := range sheets {
		byName[s.Name] = s
	}
	people := byName["People"]
	if got := people.Header(); got[0] != "realm" || got[1] != "username" || got[4] != "external_ref" {
		t.Fatalf("People header: %v", got)
	}
	if len(people.Rows) != len(out.People) {
		t.Fatalf("People rows: %d want %d", len(people.Rows), len(out.People))
	}
	grants := byName["Grants"]
	if len(grants.Rows) != len(out.Grants) || grants.Rows[0][2] != "osiris.training-admin" {
		t.Fatalf("Grants rows: %d, first %v", len(grants.Rows), grants.Rows[0])
	}
	if _, ok := byName["Memberships"]; !ok {
		t.Fatal("the Memberships sheet is missing; the importer expects it")
	}
}

func keysOf(m map[string]authzcatalog.Permission) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
