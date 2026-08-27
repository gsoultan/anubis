package feed

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

func testDialects(t *testing.T) {
	t.Helper()
	dialects = map[string]*Dialect{}
	RegisterDialect("postgres", PostgresDialect("pgx"))
	RegisterDialect("mysql", MySQLDialect("mysql"))
	RegisterDialect("sqlserver", SQLServerDialect("sqlserver"))
}

// The generated statement must be in the ENGINE'S OWN spelling. Quoting with
// the wrong character is a syntax error somebody sees; ordering with a clause
// the engine lacks is worse, because MySQL rejects NULLS FIRST outright and
// an engine that accepted it silently would hand back children before their
// parents, which the reconciler then refuses row by row.
func TestTableQueryIsBuiltInEachEnginesSpelling(t *testing.T) {
	testDialects(t)
	cfg := dbTableConfig{
		Table:   "public.customers",
		Columns: map[string]string{"ref": "id", "parent_ref": "parent_id", "name": "display_name"},
	}
	cases := []struct {
		scheme      string
		wantQuote   string
		wantOrderBy string
	}{
		{"postgres", `"public"."customers"`, "NULLS FIRST"},
		{"mysql", "`public`.`customers`", "CASE WHEN"},
		{"sqlserver", "[public].[customers]", "CASE WHEN"},
	}
	for _, c := range cases {
		d := dialects[c.scheme]
		q, err := buildTableQuery(d, cfg)
		if err != nil {
			t.Fatalf("%s: %v", c.scheme, err)
		}
		if !strings.Contains(q, c.wantQuote) {
			t.Errorf("%s: want table quoted %s, got %s", c.scheme, c.wantQuote, q)
		}
		if !strings.Contains(q, c.wantOrderBy) {
			t.Errorf("%s: want ordering via %s, got %s", c.scheme, c.wantOrderBy, q)
		}
		// Whatever the engine, the contract the reconciler reads is fixed.
		for _, alias := range []string{"AS ref", "AS name", "AS parent_ref"} {
			if !strings.Contains(q, alias) {
				t.Errorf("%s: missing %s in %s", c.scheme, alias, q)
			}
		}
	}
}

// MySQL's driver does not accept a URL. An operator should write the same
// shape of DSN whatever the engine and not have to know that.
func TestMySQLDSNIsTranslatedForItsDriver(t *testing.T) {
	testDialects(t)
	d, u, err := dialectFor("mysql://reader:s3cret@crm-db:3306/crm?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.TranslateDSN(u)
	if err != nil {
		t.Fatal(err)
	}
	const want = "reader:s3cret@tcp(crm-db:3306)/crm?parseTime=true"
	if got != want {
		t.Fatalf("translated to %q, want %q", got, want)
	}
}

// Postgres drivers take the URL unchanged; translating it would be a way to
// lose a parameter.
func TestPostgresDSNIsPassedThrough(t *testing.T) {
	testDialects(t)
	const dsn = "postgres://reader:s3cret@erp-db:5432/erp?sslmode=require"
	d, u, err := dialectFor(dsn)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := d.TranslateDSN(u)
	if got != dsn {
		t.Fatalf("got %q, want it unchanged", got)
	}
}

// An engine nobody registered must say so, and say what IS available —
// otherwise the operator is left guessing why their DSN does nothing.
func TestUnregisteredEngineNamesWhatIsAvailable(t *testing.T) {
	testDialects(t)
	_, _, err := dialectFor("oracle://scott:tiger@db:1521/orcl")
	if err == nil {
		t.Fatal("an unregistered engine was accepted")
	}
	msg := err.Error()
	if !strings.Contains(msg, "oracle") || !strings.Contains(msg, "postgres") {
		t.Fatalf("error names neither the missing engine nor the available ones: %s", msg)
	}
}

// A table or column identifier is checked before it is quoted. Quoting alone
// is not a defence anybody should rely on for a name that arrives as config.
func TestHostileIdentifiersAreRefusedNotQuoted(t *testing.T) {
	testDialects(t)
	d := dialects["postgres"]
	for _, bad := range []string{
		`customers"; DROP TABLE users; --`,
		"a.b.c",
		"1nvalid",
		"",
		strings.Repeat("x", 64),
	} {
		if _, err := quoteTable(d, bad); err == nil {
			t.Errorf("accepted hostile table name %q", bad)
		}
	}
	// And the check lives in Fetch, which is what config actually reaches.
	hostile, _ := json.Marshal(map[string]any{
		"dsn":   "postgres://u:p@erp-db:5432/d",
		"table": "customers",
		"columns": map[string]string{
			"ref":  "id",
			"name": `x" , (SELECT password FROM users) AS "y`,
		},
	})
	_, err := NewDBTableFetcher().Fetch(context.Background(),
		scopedomain.SyncSourceRecord{Kind: "db_table", Config: hostile})
	if err == nil {
		t.Fatal("a column mapping carrying SQL was accepted")
	}
	if !strings.Contains(err.Error(), "invalid identifier") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}
