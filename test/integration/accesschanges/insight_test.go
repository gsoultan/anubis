//go:build integration

package accesschanges

import (
	"context"
	"testing"

	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// The numbers the Roles list, the app page and a role's holders show are
// read from live grants: a revoked one counts for nothing, and each grant is
// sorted by what it reaches on one kind of place.
func TestUsageAndHolderKinds(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		sub := w.axis + "_sub"
		_, err := q.Exec(ctx, `INSERT INTO scope_node_types (tenant_id, code, axis_code, display_name, parent_types)
			VALUES ($1, $2, $3, 'Branch', ARRAY[$4::text])`, w.tenant, sub, w.axis, w.orgType)
		must(t, "branch level", err)
		var a1 string
		must(t, "Org A1", q.QueryRow(ctx, `SELECT scope_add_node($1, $2, $3, $4, 'org-a1', 'Org A1')::text`,
			w.tenant, w.axis, sub, w.node["Org A"]).Scan(&a1))
		w.node["Org A1"] = a1
		w.grant(t, ctx, q)                  // everything: no picks
		w.grant(t, ctx, q, "Org A")         // a branch: Org A has a place inside
		w.grant(t, ctx, q, "Org B")         // fixed: Org B has nothing inside
		gone := w.grant(t, ctx, q, "Org C") // revoked below
		_, err = q.Exec(ctx, `UPDATE grants SET revoked_at = now() WHERE id = $1`, gone)
		must(t, "revoke", err)
		_, err = q.Exec(ctx, `UPDATE roles SET application_id = $1 WHERE id = $2`, w.app, w.role)
		must(t, "role in app", err)

		k, err := repo.HolderKinds(ctx, w.tenant, w.role, w.axis)
		if err != nil {
			t.Fatal(err)
		}
		if k.Total != 3 || k.Everything != 1 || k.Branch != 1 || k.Fixed != 1 || k.Nothing != 0 {
			t.Fatalf("kinds = %+v, want 3 live: one everything, one branch, one fixed", k)
		}

		usage, err := repo.RoleUsage(ctx, w.tenant, []string{w.role})
		if err != nil {
			t.Fatal(err)
		}
		if len(usage) != 1 || usage[0].People != 1 || usage[0].ViaGroups != 0 {
			t.Fatalf("role usage = %+v, want one person, none from a group", usage)
		}
		app, err := repo.AppUsage(ctx, w.tenant, w.app)
		if err != nil {
			t.Fatal(err)
		}
		if app.People != 1 || app.Direct != 1 {
			t.Fatalf("app usage = %+v, want one person given it directly", app)
		}

		holders, err := repo.PlaceHolders(ctx, w.tenant, w.node["Org A1"], 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(holders) != 1 || !holders[0].FromAbove {
			t.Fatalf("holders of Org A1 = %+v, want the one who has Org A and everything inside", holders)
		}
		holders, err = repo.PlaceHolders(ctx, w.tenant, w.node["Org C"], 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(holders) != 0 {
			t.Fatalf("holders of Org C = %+v, want none: that grant was revoked", holders)
		}
	})
}

// A role's permission count is what it allows today, read with the role,
// so the Roles list and the give-access sheet need not fetch every role's
// permissions to say how many it has or to find one with exactly the picks.
func TestRolePermissionCount(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		roles, err := repo.ListRoles(ctx, w.tenant, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range roles {
			if r.ID == w.role {
				if r.PermissionCount != 1 {
					t.Fatalf("permission count = %d, want 1", r.PermissionCount)
				}
				return
			}
		}
		t.Fatalf("role %s not listed", w.role)
	})
}
