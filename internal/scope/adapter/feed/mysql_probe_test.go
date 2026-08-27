package feed

import (
	"context"
	"encoding/json"
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
	cfg, _ := json.Marshal(map[string]any{
		"dsn": dsn, "table": "customers",
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
