//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	gatepg "github.com/gsoultan/anubis/internal/gate/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// THE test for the gate. The gate answers from an in-memory snapshot; the
// decision endpoint answers from authorize() in SQL. Two implementations of
// one rule set is a standing invitation to drift, and drift here is not a
// wrong number on a dashboard — it is either access granted that SQL would
// deny, or a deny storm on a path SQL allows.
//
// So: load a real snapshot, replay real grants through both, and require
// EXACT agreement on every probe. math/rand/v2 is fine here — this is test
// sampling, not security (the repository-wide ban is about crypto).
func TestSnapshotAgreesWithAuthorizeEngine(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, slug := firstTenantWithSlug(ctx, t)

	repo := gatepg.New(database.New(pool))
	snap, err := repo.LoadSnapshot(ctx, tenant, slug, 30*time.Minute)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(snap.GrantsByIdentity) == 0 {
		t.Skip("no grants in this database to compare")
	}

	type probe struct {
		identity   string
		permission string
		targets    map[string]string
	}
	var probes []probe

	// Probes built from REAL grants: the permissions those grants confer and
	// the nodes they are scoped to, plus deliberate near-misses (a sibling
	// node, a missing axis) because agreement on allows proves little.
	for identity, grants := range snap.GrantsByIdentity {
		for _, g := range grants {
			var perm string
			for key := range snap.RolePermissions[g.RoleID] {
				perm = key
				break
			}
			if perm == "" {
				continue
			}
			exact := map[string]string{}
			for axis, cs := range g.Scopes {
				if len(cs) > 0 {
					exact[axis] = cs[0].NodeID
				}
			}
			probes = append(probes, probe{identity, perm, exact})

			// same permission, no targets at all: exercises fail-closed
			probes = append(probes, probe{identity, perm, map[string]string{}})

			// same permission, a descendant target: exercises inheritance
			for axis, cs := range g.Scopes {
				if len(cs) == 0 {
					continue
				}
				if child := someDescendant(ctx, cs[0].NodeID); child != "" {
					deep := map[string]string{}
					for a, c := range exact {
						deep[a] = c
					}
					deep[axis] = child
					probes = append(probes, probe{identity, perm, deep})
				}
				break
			}
			// a permission this grant does NOT confer
			probes = append(probes, probe{identity, "nonexistent:probe:action", exact})

			if len(probes) > 400 {
				break
			}
		}
		if len(probes) > 400 {
			break
		}
	}
	rand.Shuffle(len(probes), func(i, j int) { probes[i], probes[j] = probes[j], probes[i] })

	now := time.Now()
	var disagreements int
	for _, p := range probes {
		targets, _ := json.Marshal(p.targets)
		var sqlAnswer bool
		if err := pool.QueryRow(ctx, `SELECT authorize($1,$2,$3,$4::jsonb)`,
			p.identity, tenant, p.permission, string(targets)).Scan(&sqlAnswer); err != nil {
			t.Fatalf("engine: %v", err)
		}
		memAnswer := snap.Evaluate(p.identity, p.permission, p.targets, now)
		if sqlAnswer != memAnswer {
			disagreements++
			if disagreements <= 5 {
				t.Errorf("DISAGREEMENT sub=%s perm=%s targets=%s: engine=%v snapshot=%v",
					p.identity, p.permission, targets, sqlAnswer, memAnswer)
			}
		}
	}
	if disagreements > 0 {
		t.Fatalf("%d/%d probes disagreed between the SQL engine and the gate snapshot",
			disagreements, len(probes))
	}
	t.Logf("%d probes, 0 disagreements", len(probes))
}

func someDescendant(ctx context.Context, node string) string {
	var id string
	err := pool.QueryRow(ctx,
		`SELECT descendant_id FROM scope_closure
		  WHERE ancestor_id=$1 AND depth > 0 LIMIT 1`, node).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func firstTenantWithSlug(ctx context.Context, t *testing.T) (string, string) {
	t.Helper()
	var id, slug string
	if err := pool.QueryRow(ctx,
		`SELECT id, slug FROM tenants ORDER BY created_at LIMIT 1`).Scan(&id, &slug); err != nil {
		t.Skipf("no tenant: %v", err)
	}
	return id, slug
}

var _ = fmt.Sprintf
