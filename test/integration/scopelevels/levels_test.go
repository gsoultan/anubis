//go:build integration

// Package scopelevels holds a structure's level rules (migration 0055) and the
// node edits the console makes on them — restore, and search with a path — to
// the database rather than to a fake. Every test runs in a transaction that is
// rolled back, on a structure of its own.
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/integration/scopelevels/
package scopelevels

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	scopepg "github.com/gsoultan/anubis/internal/scope/adapter/postgres"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

var errRollback = errors.New("deliberate rollback")

func TestMain(m *testing.M) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		os.Exit(0) // nothing to test against; the e2e job sets this
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	pool = p
	code := m.Run()
	p.Close()
	os.Exit(code)
}

// world is a structure with Top > Company > Unit, where a company may sit in a
// company and a unit in a unit — the multinational shape.
type world struct {
	q                  database.Querier
	repo               *scopepg.Repository
	tenant, axis       string
	top, company, unit string
}

func inWorld(t *testing.T, fn func(ctx context.Context, w world)) {
	t.Helper()
	db := database.New(pool)
	err := db.WithinTx(context.Background(), func(ctx context.Context) error {
		w := world{q: db.Conn(ctx), repo: scopepg.New(db)}
		if err := w.q.QueryRow(ctx, `SELECT id::text FROM tenants ORDER BY created_at LIMIT 1`).Scan(&w.tenant); err != nil {
			t.Fatalf("no tenant — bootstrap creates one: %v", err)
		}
		w.axis = "lv_" + strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
		w.top, w.company, w.unit = w.axis+"_top", w.axis+"_co", w.axis+"_unit"
		if _, err := w.q.Exec(ctx, `INSERT INTO scope_axes (code, display_name) VALUES ($1, 'Levels test')`, w.axis); err != nil {
			t.Fatal(err)
		}
		// Children first, in ONE statement: the rules run after the statement,
		// so the order rows arrive in must not matter (bench/seed.sql).
		if _, err := w.q.Exec(ctx, `INSERT INTO scope_node_types (code, axis_code, display_name, parent_types) VALUES
			($3, $4, 'Unit', ARRAY[$2, $3]),
			($2, $4, 'Company', ARRAY[$1, $2]),
			($1, $4, 'Top', '{}')`, w.top, w.company, w.unit, w.axis); err != nil {
			t.Fatalf("levels named in any order within one statement: %v", err)
		}
		fn(ctx, w)
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}

// refused runs fn under a savepoint: a refused statement aborts the
// transaction, and the next assertion needs it alive.
func (w world) refused(t *testing.T, ctx context.Context, what string, fn func() error) {
	t.Helper()
	if _, err := w.q.Exec(ctx, "SAVEPOINT level_rule"); err != nil {
		t.Fatal(err)
	}
	err := fn()
	if _, rerr := w.q.Exec(ctx, "ROLLBACK TO SAVEPOINT level_rule"); rerr != nil {
		t.Fatal(rerr)
	}
	if err == nil {
		t.Fatalf("%s: allowed, want refused", what)
	}
}

func (w world) exec(ctx context.Context, sql string, args ...any) func() error {
	return func() error { _, err := w.q.Exec(ctx, sql, args...); return err }
}

func (w world) add(t *testing.T, ctx context.Context, typ, parent, name string) string {
	t.Helper()
	var id string
	if err := w.q.QueryRow(ctx, `SELECT scope_add_node($1, $2, $3, $4, $5, $6)::text`,
		w.tenant, w.axis, typ, parent, name, name).Scan(&id); err != nil {
		t.Fatalf("add %s: %v", name, err)
	}
	return id
}

func TestLevelRulesHoldTogether(t *testing.T) {
	inWorld(t, func(ctx context.Context, w world) {
		w.refused(t, ctx, "a second top level", w.exec(ctx,
			`INSERT INTO scope_node_types (code, axis_code, display_name) VALUES ($1, $2, 'Another top')`,
			w.axis+"_top2", w.axis))
		w.refused(t, ctx, "a parent that is no level at all", w.exec(ctx,
			`INSERT INTO scope_node_types (code, axis_code, display_name, parent_types) VALUES ($1, $2, 'Desk', '{nowhere_at_all}')`,
			w.axis+"_desk", w.axis))
		// Another structure's level exists, and is still not a parent here.
		// Built here: a freshly bootstrapped database has no other structure,
		// and a test that borrows one passes or fails on what ran before it.
		other, foreign := w.axis+"x", w.axis+"x_top"
		if _, err := w.q.Exec(ctx, `INSERT INTO scope_axes (code, display_name) VALUES ($1, 'Another structure')`, other); err != nil {
			t.Fatal(err)
		}
		if _, err := w.q.Exec(ctx, `INSERT INTO scope_node_types (code, axis_code, display_name) VALUES ($1, $2, 'Its top')`, foreign, other); err != nil {
			t.Fatalf("a second structure's own top level: %v", err)
		}
		w.refused(t, ctx, "a parent from another structure", w.exec(ctx,
			`INSERT INTO scope_node_types (code, axis_code, display_name, parent_types) VALUES ($1, $2, 'Desk', ARRAY[$3])`,
			w.axis+"_desk", w.axis, foreign))
		w.refused(t, ctx, "giving the top level a parent", w.exec(ctx,
			`UPDATE scope_node_types SET parent_types = ARRAY[$2] WHERE code = $1`, w.top, w.company))
		w.refused(t, ctx, "taking every parent from a level", w.exec(ctx,
			`UPDATE scope_node_types SET parent_types = '{}' WHERE code = $1`, w.unit))
		w.refused(t, ctx, "moving a level to another structure", w.exec(ctx,
			`UPDATE scope_node_types SET axis_code = $2 WHERE code = $1`, w.unit, other))
		w.refused(t, ctx, "deleting a level another level sits under", w.exec(ctx,
			`DELETE FROM scope_node_types WHERE code = $1`, w.company))

		// What stays legal: renaming, a level inside itself, the top's name.
		if _, err := w.q.Exec(ctx, `UPDATE scope_node_types SET display_name = 'Business unit' WHERE code = $1`, w.unit); err != nil {
			t.Fatalf("renaming a level: %v", err)
		}
		if _, err := w.q.Exec(ctx, `UPDATE scope_node_types SET display_name = 'Group' WHERE code = $1`, w.top); err != nil {
			t.Fatalf("renaming the top level: %v", err)
		}
	})
}

// A rule an item relies on cannot be taken away — archived items included,
// because restoring one does not re-run the placement guard. One that nothing
// relies on can.
func TestLevelRuleInUseStays(t *testing.T) {
	inWorld(t, func(ctx context.Context, w world) {
		var root string
		if err := w.q.QueryRow(ctx, `SELECT scope_ensure_root($1, $2)::text`, w.tenant, w.axis).Scan(&root); err != nil {
			t.Fatal(err)
		}
		group := w.add(t, ctx, w.company, root, "Group")
		sub := w.add(t, ctx, w.company, group, "Subsidiary") // a company in a company
		w.add(t, ctx, w.unit, sub, "Sales")

		if err := w.repo.UpdateScopeNodeType(ctx, scopedomain.ScopeNodeTypeRecord{
			Code: w.unit, Axis: w.axis, DisplayName: "Unit", ParentTypes: []string{w.company},
		}); err != nil {
			t.Fatalf("removing the unit-in-unit rule nobody uses: %v", err)
		}
		if _, err := w.q.Exec(ctx, `UPDATE scope_nodes SET status = 'archived' WHERE id = $1`, sub); err != nil {
			t.Fatal(err)
		}
		w.refused(t, ctx, "removing company-in-company while an archived company sits in one", func() error {
			return w.repo.UpdateScopeNodeType(ctx, scopedomain.ScopeNodeTypeRecord{
				Code: w.company, Axis: w.axis, DisplayName: "Company", ParentTypes: []string{w.top},
			})
		})
		err := w.repo.UpdateScopeNodeType(ctx, scopedomain.ScopeNodeTypeRecord{
			Code: w.axis + "_nope", Axis: w.axis, DisplayName: "Nope", ParentTypes: []string{w.top},
		})
		if !errors.Is(err, apperr.ErrNotFound) && apperr.AsError(err).Code != apperr.ErrNotFound.Code {
			t.Fatalf("editing a level that does not exist = %v, want not found", err)
		}
	})
}

func TestRestoreAndSearchPath(t *testing.T) {
	inWorld(t, func(ctx context.Context, w world) {
		var root string
		if err := w.q.QueryRow(ctx, `SELECT scope_ensure_root($1, $2)::text`, w.tenant, w.axis).Scan(&root); err != nil {
			t.Fatal(err)
		}
		// A top item is named after its level, not 'All <axis code>'.
		if top, err := w.repo.ScopeNode(ctx, w.tenant, root); err != nil || top.Name != "Top" {
			t.Fatalf("top item = %+v, %v; want it named after its level", top, err)
		}
		acme := w.add(t, ctx, w.company, root, "Acme Group")
		asia := w.add(t, ctx, w.company, acme, "Acme Asia")
		sales := w.add(t, ctx, w.unit, asia, "Sales lv")
		w.add(t, ctx, w.unit, acme, "Sales lv HQ")

		hits, err := w.repo.ListScopeNodes(ctx, w.tenant, scopedomain.ScopeNodeFilter{Axis: w.axis, Query: "sales lv"})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 2 {
			t.Fatalf("search found %d, want the 2 Sales", len(hits))
		}
		paths := map[string][]string{}
		for _, h := range hits {
			paths[h.ID] = h.Path
		}
		if got := paths[sales]; !slices.Equal(got, []string{"Acme Group", "Acme Asia"}) {
			t.Fatalf("path of the Asian Sales = %q, want top first and no structure root", got)
		}
		page, err := w.repo.ListScopeNodes(ctx, w.tenant, scopedomain.ScopeNodeFilter{Axis: w.axis, ParentID: asia})
		if err != nil || len(page) != 1 {
			t.Fatalf("children of Asia = %d, %v", len(page), err)
		}
		if len(page[0].Path) != 0 {
			t.Fatalf("a tree listing paid for a path it does not show: %q", page[0].Path)
		}

		if err := w.repo.RestoreScopeNode(ctx, w.tenant, sales); !errors.Is(err, apperr.ErrNotFound) &&
			apperr.AsError(err).Code != apperr.ErrNotFound.Code {
			t.Fatalf("restoring an item that is not archived = %v, want not found", err)
		}
		if err := w.repo.ArchiveScopeNode(ctx, w.tenant, sales); err != nil {
			t.Fatal(err)
		}
		var other string
		if err := w.q.QueryRow(ctx, `SELECT id::text FROM tenants WHERE id <> $1 LIMIT 1`, w.tenant).Scan(&other); err == nil {
			if err := w.repo.RestoreScopeNode(ctx, other, sales); err == nil {
				t.Fatal("another tenant restored this tenant's item")
			}
		}
		if err := w.repo.RestoreScopeNode(ctx, w.tenant, sales); err != nil {
			t.Fatalf("restore: %v", err)
		}
		n, err := w.repo.ScopeNode(ctx, w.tenant, sales)
		if err != nil || n.Status != "active" {
			t.Fatalf("after restore: %+v, %v", n, err)
		}
	})
}
