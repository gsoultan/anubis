//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// ROADMAP CLAIM (unproven): "scope_move_node is safe under concurrency —
// asserts SERIALIZABLE; not stress-tested with concurrent moves in the same
// subtree."
//
// The danger is the closure table: two moves interleaving can leave a node
// reachable from an ancestor it no longer descends from, which is a silent
// PRIVILEGE LEAK — a grant at the old parent keeps deciding for a subtree
// that moved away. This hammers concurrent moves within one subtree and then
// checks the closure against the parent pointers, which are the truth.
func TestConcurrentSubtreeMovesKeepClosureConsistent(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, axis := setupMoveFixture(ctx, t)

	// Fetch the movable nodes and the two candidate parents.
	var nodes []string
	rows, err := pool.Query(ctx,
		`SELECT id FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2 AND slug LIKE 'leaf-%' ORDER BY slug`,
		tenant, axis)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, id)
	}
	rows.Close()

	var parents []string
	prows, err := pool.Query(ctx,
		`SELECT id FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2 AND slug LIKE 'branch-%' ORDER BY slug`,
		tenant, axis)
	if err != nil {
		t.Fatal(err)
	}
	for prows.Next() {
		var id string
		if err := prows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		parents = append(parents, id)
	}
	prows.Close()

	if len(nodes) < 4 || len(parents) < 2 {
		t.Fatalf("fixture: %d leaves, %d branches", len(nodes), len(parents))
	}

	// Every worker moves every leaf between the two branches, repeatedly.
	var wg sync.WaitGroup
	var serialisationFailures int
	var mu sync.Mutex
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i, n := range nodes {
				target := parents[(w+i)%len(parents)]
				if err := moveSerializable(ctx, n, target); err != nil {
					// 40001 is expected and correct: SERIALIZABLE told the
					// caller to retry. A retry loop is the application's job.
					if strings.Contains(err.Error(), "40001") || strings.Contains(err.Error(), "40P01") {
						mu.Lock()
						serialisationFailures++
						mu.Unlock()
						continue
					}
					mu.Lock()
					t.Errorf("worker %d move: %v", w, err)
					mu.Unlock()
					return
				}
			}
		}(w)
	}
	wg.Wait()
	t.Logf("serialisation retries observed: %d", serialisationFailures)

	assertClosureMatchesTree(ctx, t, tenant, axis)
}

func moveSerializable(ctx context.Context, node, parent string) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT scope_move_node($1,$2)`, node, parent); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// assertClosureMatchesTree recomputes reachability from parent_id — the
// authoritative structure — and compares it with scope_closure. Any
// difference is exactly the leak this test exists to catch.
func assertClosureMatchesTree(ctx context.Context, t *testing.T, tenant, axis string) {
	t.Helper()
	const q = `
WITH RECURSIVE tree AS (
    SELECT id AS ancestor_id, id AS descendant_id, 0 AS depth
      FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2
    UNION ALL
    SELECT t.ancestor_id, n.id, t.depth+1
      FROM tree t JOIN scope_nodes n ON n.parent_id = t.descendant_id
     WHERE n.tenant_id=$1 AND n.axis_code=$2
),
actual AS (
    SELECT c.ancestor_id, c.descendant_id FROM scope_closure c
      JOIN scope_nodes d ON d.id = c.descendant_id
     WHERE d.tenant_id=$1 AND d.axis_code=$2
)
SELECT
  (SELECT count(*) FROM (SELECT ancestor_id,descendant_id FROM tree
      EXCEPT SELECT ancestor_id,descendant_id FROM actual) x) AS missing,
  (SELECT count(*) FROM (SELECT ancestor_id,descendant_id FROM actual
      EXCEPT SELECT ancestor_id,descendant_id FROM tree) y) AS extra`
	var missing, extra int
	if err := pool.QueryRow(ctx, q, tenant, axis).Scan(&missing, &extra); err != nil {
		t.Fatal(err)
	}
	if missing != 0 || extra != 0 {
		t.Fatalf("closure diverged from the tree after concurrent moves: %d missing, %d EXTRA "+
			"(extra rows are the privilege-leak case: a grant above a node that no longer descends from it)",
			missing, extra)
	}
}

func setupMoveFixture(ctx context.Context, t *testing.T) (tenant, axis string) {
	t.Helper()
	axis = "movetest"
	if err := pool.QueryRow(ctx,
		`SELECT id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenant); err != nil {
		t.Skipf("no tenant seeded: %v", err)
	}
	exec := func(sql string, args ...any) {
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	exec(`INSERT INTO scope_axes (code, display_name) VALUES ($1,'Move Test')
	      ON CONFLICT (code) DO NOTHING`, axis)
	exec(`INSERT INTO scope_node_types (code, axis_code, display_name, parent_types)
	      VALUES ('mv_root',$1,'Root','{}') ON CONFLICT (code) DO NOTHING`, axis)
	exec(`INSERT INTO scope_node_types (code, axis_code, display_name, parent_types)
	      VALUES ('mv_node',$1,'Node','{mv_root,mv_node}') ON CONFLICT (code) DO NOTHING`, axis)

	var root string
	if err := pool.QueryRow(ctx, `SELECT scope_ensure_root($1,$2)`, tenant, axis).Scan(&root); err != nil {
		t.Fatal(err)
	}
	// Two branches under the root, four leaves under the first branch.
	branches := make([]string, 2)
	for i := range branches {
		slug := fmt.Sprintf("branch-%d", i)
		if err := pool.QueryRow(ctx,
			`SELECT COALESCE((SELECT id FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2 AND slug=$3),
			                 scope_add_node($1,$2,'mv_node',$4,$3,$3,NULL))`,
			tenant, axis, slug, root).Scan(&branches[i]); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		slug := fmt.Sprintf("leaf-%d", i)
		var id string
		if err := pool.QueryRow(ctx,
			`SELECT COALESCE((SELECT id FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2 AND slug=$3),
			                 scope_add_node($1,$2,'mv_node',$4,$3,$3,NULL))`,
			tenant, axis, slug, branches[0]).Scan(&id); err != nil {
			t.Fatal(err)
		}
	}
	return tenant, axis
}
