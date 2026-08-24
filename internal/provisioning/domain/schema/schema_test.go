package schema

import (
	"strings"
	"testing"
)

// Index falls back to matching headers with separators removed. That is
// only safe while no two columns on a sheet collide once stripped — this
// test is what stops a future column called "scope ref" from quietly
// hijacking "scope_ref".
func TestStrippedColumnKeysDoNotCollide(t *testing.T) {
	for _, s := range Workbook() {
		seen := map[string]string{}
		for _, c := range s.Columns {
			k := strip(normalize(c.Key))
			if prev, dup := seen[k]; dup {
				t.Errorf("sheet %s: %q and %q are indistinguishable once separators are stripped",
					s.Name, prev, c.Key)
			}
			seen[k] = c.Key
		}
	}
}

func TestEveryColumnIsDocumented(t *testing.T) {
	for _, s := range Workbook() {
		if s.Purpose == "" {
			t.Errorf("sheet %s has no purpose text", s.Name)
		}
		for _, c := range s.Columns {
			if c.Help == "" {
				t.Errorf("%s.%s has no help text — it ships in the template", s.Name, c.Key)
			}
			if c.Key != strings.ToLower(c.Key) {
				t.Errorf("%s.%s should be lower_snake_case", s.Name, c.Key)
			}
		}
	}
}

// A spreadsheet of plaintext passwords gets mailed around as an
// attachment. If someone adds a password column, this should stop them.
func TestTemplateCarriesNoSecretColumns(t *testing.T) {
	for _, s := range Workbook() {
		for _, c := range s.Columns {
			for _, banned := range []string{"password", "secret", "token", "otp", "pin"} {
				if strings.Contains(c.Key, banned) {
					t.Errorf("%s.%s puts a %s in a spreadsheet", s.Name, c.Key, banned)
				}
			}
		}
	}
}

func TestSheetLookupIsCaseInsensitive(t *testing.T) {
	for _, name := range []string{"People", "people", "PEOPLE", " people "} {
		if _, ok := Sheet(name); !ok {
			t.Errorf("Sheet(%q) not found", name)
		}
	}
	if _, ok := Sheet("Instructions"); ok {
		t.Error("Instructions is documentation, not a data sheet")
	}
}

func TestIndexReportsMissingRequiredColumns(t *testing.T) {
	spec, _ := Sheet(SheetGrants)
	_, missing := Index(spec, []string{"realm", "username"})
	if len(missing) != 1 || missing[0] != ColRole {
		t.Fatalf("missing = %v, want [role]", missing)
	}
}

func TestValueToleratesShortRows(t *testing.T) {
	spec, _ := Sheet(SheetPeople)
	idx, missing := Index(spec, spec.Keys())
	if len(missing) != 0 {
		t.Fatalf("template header should satisfy its own schema: %v", missing)
	}
	if got := idx.Value([]string{"staff"}, ColEmail); got != "" {
		t.Errorf("short row read %q, want empty", got)
	}
	if got := idx.Value([]string{"  staff  "}, ColRealm); got != "staff" {
		t.Errorf("value = %q, want trimmed", got)
	}
}
