//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// Before migration 0038 the depth ceiling announced itself as
// "ERROR: smallint out of range" — no table, no node, no operation. During a
// sync from an external system that is indistinguishable from an overflow
// anywhere else in the transaction, and the actual cause (a self-referencing
// feed building an unbounded chain) is nowhere in the message.
//
// The real ceiling is 32,767, which cannot be reached in a test: the closure
// is quadratic, so that chain is ~537M rows. scope_max_depth() is a FUNCTION
// rather than a literal precisely so the limit can be lowered inside a
// transaction and rolled back, which is what these tests do.
//
// Everything below builds its own nodes inside the transaction. The shared
// fixture cannot be trusted for depths: TestConcurrentSubtreeMovesKeepClosure
// Consistent reparents its leaves, so their depth depends on what ran before.

// addIn creates a node inside tx and returns its id.
func addIn(ctx context.Context, t *testing.T, tx pgx.Tx, tenant, axis, slug, parent string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx,
		`SELECT scope_add_node($1,$2,'mv_node',$3,$4,$4,NULL)`,
		tenant, axis, parent, slug).Scan(&id); err != nil {
		t.Fatalf("add %s: %v", slug, err)
	}
	return id
}

func capDepthIn(ctx context.Context, t *testing.T, tx pgx.Tx, limit int) {
	t.Helper()
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`CREATE OR REPLACE FUNCTION scope_max_depth() RETURNS integer
		 LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $fn$ SELECT %d $fn$`, limit)); err != nil {
		t.Fatal(err)
	}
}

func axisRoot(ctx context.Context, t *testing.T, tx pgx.Tx, tenant, axis string) string {
	t.Helper()
	var root string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2 AND is_axis_root`,
		tenant, axis).Scan(&root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestScopeDepthLimitIsNamedNotATypeError(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, axis := setupMoveFixture(ctx, t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	root := axisRoot(ctx, t, tx, tenant, axis)
	a := addIn(ctx, t, tx, tenant, axis, "dl-a", root) // depth 1
	capDepthIn(ctx, t, tx, 1)

	// depth 2 under a ceiling of 1.
	_, err = tx.Exec(ctx, `SELECT scope_add_node($1,$2,'mv_node',$3,'dl-too-deep','Too Deep',NULL)`,
		tenant, axis, a)
	if err == nil {
		t.Fatal("adding past the depth ceiling succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("not a postgres error: %v", err)
	}
	// 54000 is program_limit_exceeded. The point of the test is that this is
	// no longer 22003 (numeric_value_out_of_range) from a smallint overflow.
	if pgErr.Code != "54000" {
		t.Errorf("SQLSTATE = %s, want 54000 (program_limit_exceeded): %q", pgErr.Code, pgErr.Message)
	}
	for _, want := range []string{"too deep", "dl-too-deep", axis} {
		if !strings.Contains(pgErr.Message, want) {
			t.Errorf("message %q does not mention %q", pgErr.Message, want)
		}
	}
}

// scope_move_node overflows where neither side is near the limit on its own:
// the resulting depth is the new parent's depth PLUS the moving subtree's
// height. A guard that only looked at one of them would miss this.
func TestScopeMoveRespectsTheDepthLimit(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, axis := setupMoveFixture(ctx, t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	root := axisRoot(ctx, t, tx, tenant, axis)
	a := addIn(ctx, t, tx, tenant, axis, "dm-a", root) // depth 1
	b := addIn(ctx, t, tx, tenant, axis, "dm-b", a)    // depth 2  <- destination
	x := addIn(ctx, t, tx, tenant, axis, "dm-x", root) // depth 1, height 1
	addIn(ctx, t, tx, tenant, axis, "dm-y", x)         // depth 2

	// Ceiling 3: the destination is at 2 and the subtree is 1 tall, so each
	// half is legal and the graft (2 + 1 + 1 = 4) is not.
	capDepthIn(ctx, t, tx, 3)

	// The RAISE aborts the transaction, so the expected failure runs inside a
	// savepoint — otherwise the legal-move assertion below cannot run.
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `SELECT scope_move_node($1,$2)`, x, b)
	if err == nil {
		t.Fatal("moving a subtree past the depth ceiling succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "54000" {
		t.Fatalf("want SQLSTATE 54000, got %v", err)
	}
	if !strings.Contains(pgErr.Message, "too deep") {
		t.Errorf("message %q does not say what went wrong", pgErr.Message)
	}
	if err := sp.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	// And the same move under a ceiling that accommodates it must still work.
	capDepthIn(ctx, t, tx, 4)
	if _, err := tx.Exec(ctx, `SELECT scope_move_node($1,$2)`, x, b); err != nil {
		t.Fatalf("a legal move was rejected: %v", err)
	}
}

// Keyset paging orders by (name, id). The id is not decoration: scope node
// names are NOT unique — a sync from an ERP happily produces twenty offices
// called "Warehouse" — and a cursor of name alone cannot distinguish the rows
// inside such a run. Resuming after the name either re-reads the whole run
// (the walk never terminates) or skips past it (nodes silently vanish from
// the archive pass). This builds a run of identical names longer than the
// page and requires every node exactly once.
func TestKeysetPagingSurvivesDuplicateNames(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, axis := setupMoveFixture(ctx, t)

	const dupes = 25
	const pageSize = 10

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	root := axisRoot(ctx, t, tx, tenant, axis)
	parent := addIn(ctx, t, tx, tenant, axis, "dup-parent", root)
	want := map[string]bool{}
	for i := 0; i < dupes; i++ {
		var id string
		// Same NAME for every row; only the slug differs.
		if err := tx.QueryRow(ctx,
			`SELECT scope_add_node($1,$2,'mv_node',$3,$4,'Warehouse',NULL)`,
			tenant, axis, parent, fmt.Sprintf("dup-%02d", i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		want[id] = true
	}

	// Page through the children with the same keyset the repository uses.
	seen := map[string]int{}
	afterName, afterID := "", ""
	for page := 0; ; page++ {
		if page > dupes { // a non-advancing cursor would spin here forever
			t.Fatalf("paging did not terminate after %d pages", page)
		}
		rows, qerr := tx.Query(ctx, `
			SELECT id, name FROM scope_nodes
			 WHERE tenant_id=$1 AND axis_code=$2 AND parent_id=$3 AND status='active'
			   AND ($4::text IS NULL
			        OR name > $4::text
			        OR (name = $4::text AND id > $5::uuid))
			 ORDER BY name, id
			 LIMIT $6`,
			tenant, axis, parent,
			nullIfEmpty(afterName), nullIfEmpty(afterID), pageSize)
		if qerr != nil {
			t.Fatal(qerr)
		}
		n := 0
		var lastName, lastID string
		for rows.Next() {
			var id, name string
			if serr := rows.Scan(&id, &name); serr != nil {
				rows.Close()
				t.Fatal(serr)
			}
			seen[id]++
			lastName, lastID = name, id
			n++
		}
		rows.Close()
		if rerr := rows.Err(); rerr != nil {
			t.Fatal(rerr)
		}
		if n < pageSize {
			break
		}
		afterName, afterID = lastName, lastID
	}

	if len(seen) != dupes {
		t.Errorf("saw %d distinct nodes, want %d", len(seen), dupes)
	}
	for id := range want {
		switch seen[id] {
		case 1:
		case 0:
			t.Errorf("node %s was never returned — paging skipped it", id)
		default:
			t.Errorf("node %s was returned %d times — paging repeated it", id, seen[id])
		}
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
