//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ROADMAP CLAIM (unproven): "role_recompute_effective handles deep role
// graphs — CYCLE detection is present; not tested beyond shallow graphs."
//
// Two failure modes matter. A deep chain must actually propagate permissions
// all the way down (a truncated walk silently DENIES access people were
// granted). A cycle must terminate rather than recurse forever (a hung
// recompute holds a transaction open on the hot catalog).
func TestDeepRoleGraphRecompute(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	appID, appSlug := firstApplication(ctx, t, tenant)

	// One permission at the top of a 60-deep inheritance chain.
	var permID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action, description)
		 VALUES ($1,$2,$3,'deepgraph','probe','depth probe')
		 ON CONFLICT (application_id, resource, action) DO UPDATE SET description = EXCLUDED.description
		 RETURNING id`, appID, tenant, appSlug).Scan(&permID); err != nil {
		t.Fatal(err)
	}

	const depth = 60
	roles := make([]string, depth)
	for i := 0; i < depth; i++ {
		name := fmt.Sprintf("deepgraph.level%02d", i)
		if err := pool.QueryRow(ctx,
			`INSERT INTO roles (tenant_id, name, description, allowed_realm_kinds)
			 VALUES ($1,$2,'depth probe','{internal}')
			 ON CONFLICT (tenant_id, name) DO UPDATE SET description = EXCLUDED.description
			 RETURNING id`, tenant, name).Scan(&roles[i]); err != nil {
			t.Fatal(err)
		}
	}
	// level0 holds the permission; each level inherits from the one above.
	if _, err := pool.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1,$2)
		 ON CONFLICT DO NOTHING`, roles[0], permID); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < depth; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO role_parents (role_id, parent_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			roles[i], roles[i-1]); err != nil {
			t.Fatal(err)
		}
	}

	// The deepest role must end up holding the permission from 60 levels up.
	start := time.Now()
	if _, err := pool.Exec(ctx, `SELECT role_recompute_effective($1)`, roles[depth-1]); err != nil {
		t.Fatalf("recompute at depth %d: %v", depth, err)
	}
	elapsed := time.Since(start)

	var holds bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM role_permissions_effective
		                 WHERE role_id=$1 AND permission_id=$2)`,
		roles[depth-1], permID).Scan(&holds); err != nil {
		t.Fatal(err)
	}
	if !holds {
		t.Fatalf("permission did not propagate %d levels — inheritance truncates, "+
			"which silently denies access that was granted", depth)
	}
	t.Logf("depth %d propagated in %s", depth, elapsed)
}

// A cycle must terminate. Postgres CYCLE detection is what makes this safe;
// without it the recompute never returns and holds the catalog hostage.
func TestCyclicRoleGraphTerminates(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)

	ids := make([]string, 3)
	for i := range ids {
		name := fmt.Sprintf("cyclegraph.node%d", i)
		if err := pool.QueryRow(ctx,
			`INSERT INTO roles (tenant_id, name, description, allowed_realm_kinds)
			 VALUES ($1,$2,'cycle probe','{internal}')
			 ON CONFLICT (tenant_id,name) DO UPDATE SET description = EXCLUDED.description
			 RETURNING id`, tenant, name).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	// 0 -> 1 -> 2 -> 0
	for i := range ids {
		if _, err := pool.Exec(ctx,
			`INSERT INTO role_parents (role_id, parent_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			ids[i], ids[(i+1)%len(ids)]); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan error, 1)
	go func() {
		_, err := pool.Exec(ctx, `SELECT role_recompute_effective($1)`, ids[0])
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cyclic recompute errored instead of terminating: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("recompute did not terminate on a cyclic role graph — CYCLE detection is not working")
	}
}

func firstTenant(ctx context.Context, t *testing.T) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("no tenant seeded: %v", err)
	}
	return id
}

func firstApplication(ctx context.Context, t *testing.T, tenant string) (string, string) {
	t.Helper()
	var id, slug string
	if err := pool.QueryRow(ctx,
		`SELECT id, slug FROM applications WHERE tenant_id=$1 ORDER BY created_at LIMIT 1`,
		tenant).Scan(&id, &slug); err != nil {
		t.Skipf("no application seeded: %v", err)
	}
	return id, slug
}
