package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"testing"

	// The package under test links no driver — that is the point of the
	// registry. A test that wants MySQL brings it, exactly as the
	// composition root does.
	_ "github.com/go-sql-driver/mysql"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

// Proves "any database" is real rather than an abstraction nobody exercised:
// this reads a structure out of MySQL, whose driver takes a different DSN,
// quotes with backticks, and has no NULLS FIRST.
func TestReadsAStructureOutOfMySQL(t *testing.T) {
	dsn := os.Getenv("ANUBIS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ANUBIS_TEST_MYSQL_DSN not set")
	}
	testDialects(t)
	table := seedCustomers(t, dsn)
	cfg, _ := json.Marshal(map[string]any{
		"dsn": dsn, "table": table,
		"columns": map[string]string{
			"ref": "id", "parent_ref": "parent_id",
			"name": "display_name", "node_type": "kind",
		},
	})
	rows, err := NewDBTableFetcher().Fetch(context.Background(),
		scopedomain.SyncSourceRecord{Kind: "db_table", Config: cfg})
	if err != nil {
		t.Fatalf("mysql fetch: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	// The table lists the child first. The engine has no NULLS FIRST, so the
	// dialect's CASE expression is what puts the root ahead of it — and the
	// reconciler refuses a child whose parent it has not seen.
	if rows[0].Ref != "C-BRANCH" {
		t.Fatalf("root did not sort first: %+v", rows)
	}
	if rows[1].ParentRef != "C-BRANCH" || rows[1].NodeType != "e2e_my_node" {
		t.Fatalf("column mapping wrong: %+v", rows[1])
	}
	t.Logf("mysql: %s(%s) -> %s(%s)", rows[0].Ref, rows[0].NodeType, rows[1].Ref, rows[1].ParentRef)
}

// The fixture the probe reads. It lives in the test so the test is true
// wherever ANUBIS_TEST_MYSQL_DSN points, instead of only on a machine where
// someone once typed the CREATE TABLE by hand.
//
// C-LEAF is inserted first and sorts first by primary key, so natural row
// order puts the child ahead of its parent: whether the root comes back first
// is decided by the dialect, not by luck.
func seedCustomers(t *testing.T, dsn string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("ANUBIS_TEST_MYSQL_DSN is not a URL: %v", err)
	}
	db, err := sql.Open("mysql", u.User.String()+"@tcp("+u.Host+")"+u.Path)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer db.Close()

	const table = "anubis_probe_customers"
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS ` + table + ` (
		   id VARCHAR(32) PRIMARY KEY, parent_id VARCHAR(32) NULL,
		   display_name VARCHAR(128), kind VARCHAR(64))`,
		`INSERT IGNORE INTO ` + table + ` VALUES
		   ('C-LEAF','C-BRANCH','Leaf Customer','e2e_my_node'),
		   ('C-BRANCH',NULL,'Branch Customer','e2e_my_root')`,
	} {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed mysql: %v", err)
		}
	}
	return table
}
