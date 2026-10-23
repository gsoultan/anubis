//go:build integration

package accesschanges

import (
	"context"
	"testing"

	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	"github.com/gsoultan/anubis/internal/authz/domain/grantchange"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// plan runs a change the way the console does: candidates and facts from
// the database, the plan from the pure planner.
func plan(t *testing.T, ctx context.Context, repo *authzpg.Repository, w world, s grantchange.Spec) grantchange.Plan {
	t.Helper()
	var f grantchange.AxisFacts
	deny, root, err := repo.AxisFacts(ctx, w.tenant, w.axis)
	if err != nil {
		t.Fatal(err)
	}
	f.Deny, f.RootID = deny, root
	if s.SiblingsOf != "" {
		_, sib, _, err := repo.NodeFamily(ctx, w.tenant, s.SiblingsOf)
		if err != nil {
			t.Fatal(err)
		}
		f.Siblings = sib
	}
	cands, err := repo.ChangeCandidates(ctx, w.tenant, s.RoleID, grantchange.MaxGrants+1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := grantchange.Build(s, cands, f)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// A new organisation is added. The person whose list named every other one
// is found by "siblings of", given it in one change, and authorize() agrees;
// undoing the change takes it away again.
func TestAddingAPlaceToFullListsAndUndoingIt(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		w.grant(t, ctx, q, "Org A", "Org B", "Org C")
		// One with a partial list, which must not be touched.
		partial := w.grant(t, ctx, q, "Org A")
		d := w.add(t, ctx, q, "Org D")
		if w.allows(t, ctx, q, "Org D") {
			t.Fatal("fixture: Org D allowed before any change")
		}

		s := grantchange.Spec{RoleID: w.role, Axis: w.axis, SiblingsOf: d,
			Kind: grantchange.AddPlace, NodeID: d, Inherit: true, Reason: "new org"}
		p := plan(t, ctx, repo, w, s)
		if p.Matched != 1 || len(p.Items) != 1 {
			t.Fatalf("plan matched %d, changes %d; want the one full list", p.Matched, len(p.Items))
		}
		id, err := repo.RecordGrantChange(ctx, w.tenant, w.person, s, p.Items)
		if err != nil {
			t.Fatal(err)
		}
		commitChecks(t, ctx, q)
		if !w.allows(t, ctx, q, "Org D") {
			t.Fatal("after the change: Org D still refused")
		}
		var picks int
		must(t, "count", q.QueryRow(ctx, `SELECT count(*) FROM grant_scopes WHERE grant_id = $1`, partial).Scan(&picks))
		if picks != 1 {
			t.Fatalf("the partial list was changed: %d picks", picks)
		}

		rec, items, err := repo.GrantChange(ctx, w.tenant, id)
		if err != nil || rec.Grants != 1 || len(items) != 1 {
			t.Fatalf("recorded change = %+v, %d items, %v", rec, len(items), err)
		}
		now, err := repo.CurrentStates(ctx, w.tenant, []string{items[0].GrantID})
		if err != nil {
			t.Fatal(err)
		}
		if !now[items[0].GrantID].Same(items[0].After) {
			t.Fatalf("grant does not read back as the change left it:\n now %+v\nafter %+v", now[items[0].GrantID], items[0].After)
		}
		if err := repo.RestoreGrantChange(ctx, w.tenant, id, w.person, items); err != nil {
			t.Fatal(err)
		}
		commitChecks(t, ctx, q)
		if w.allows(t, ctx, q, "Org D") {
			t.Fatal("after undo: Org D still allowed")
		}
		if !w.allows(t, ctx, q, "Org A") {
			t.Fatal("after undo: Org A lost")
		}
		rec, _, _ = repo.GrantChange(ctx, w.tenant, id)
		if rec.UndoneAt == nil {
			t.Fatal("undo did not mark the change undone")
		}
	})
}

// Growing a list with an exception to "everything" has to keep an include
// beside the exception, or the exclusion guard refuses it at COMMIT.
func TestGrowingAListWithAnExceptionKeepsTheException(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		g := w.grant(t, ctx, q, "Org A")
		_, err := q.Exec(ctx, `INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit, mode)
			VALUES ($1, $2, $3, $4, true, 'exclude')`, g, w.tenant, w.axis, w.node["Org C"])
		must(t, "exception", err)
		s := grantchange.Spec{RoleID: w.role, Axis: w.axis, Kind: grantchange.Grow}
		p := plan(t, ctx, repo, w, s)
		if len(p.Items) != 1 {
			t.Fatalf("plan = %+v", p)
		}
		if _, err := repo.RecordGrantChange(ctx, w.tenant, w.person, s, p.Items); err != nil {
			t.Fatal(err)
		}
		commitChecks(t, ctx, q)
		if !w.allows(t, ctx, q, "Org B") || w.allows(t, ctx, q, "Org C") {
			t.Fatal("grown list: want Org B allowed and Org C still left out")
		}
	})
}

// A change that ends access records why, and undo brings it back.
func TestRevokeInBulkAndUndo(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		w.grant(t, ctx, q, "Org A")
		s := grantchange.Spec{RoleID: w.role, Kind: grantchange.Revoke, Reason: "left the team"}
		p := plan(t, ctx, repo, w, s)
		id, err := repo.RecordGrantChange(ctx, w.tenant, w.person, s, p.Items)
		if err != nil {
			t.Fatal(err)
		}
		if w.allows(t, ctx, q, "Org A") {
			t.Fatal("revoked in bulk, still allowed")
		}
		var why string
		must(t, "reason", q.QueryRow(ctx, `SELECT revoke_reason FROM grants WHERE id = $1`, p.Items[0].GrantID).Scan(&why))
		if why != "left the team" {
			t.Fatalf("revoke_reason = %q", why)
		}
		_, items, _ := repo.GrantChange(ctx, w.tenant, id)
		if err := repo.RestoreGrantChange(ctx, w.tenant, id, w.person, items); err != nil {
			t.Fatal(err)
		}
		if !w.allows(t, ctx, q, "Org A") {
			t.Fatal("undo of a revoke did not bring the access back")
		}
	})
}

// The insight queries: the full list is found, the new place's reach names
// the role, and the person's summary names it too.
func TestFullListsAndPlaceReach(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		w.grant(t, ctx, q, "Org A", "Org B", "Org C")
		lists, err := repo.ListFullLists(ctx, w.tenant, w.axis, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(lists) != 1 || lists[0].RoleID != w.role || !lists[0].IsRoot || lists[0].Places != 3 || lists[0].Grants != 1 {
			t.Fatalf("full lists = %+v", lists)
		}
		d := w.add(t, ctx, q, "Org D")
		reach, err := repo.PlaceReach(ctx, w.tenant, d)
		if err != nil {
			t.Fatal(err)
		}
		if len(reach.MightExpect) != 1 || reach.MightExpect[0].RoleID != w.role || reach.MightExpect[0].People != 1 {
			t.Fatalf("might expect = %+v", reach.MightExpect)
		}
		if lists, _ = repo.ListFullLists(ctx, w.tenant, w.axis, ""); len(lists) != 0 {
			t.Fatalf("after Org D the list is no longer full, got %+v", lists)
		}
		sums, err := repo.SummarizeAccess(ctx, w.tenant, []string{w.person})
		if err != nil || len(sums) != 1 || sums[0].Direct < 1 {
			t.Fatalf("summary = %+v, %v", sums, err)
		}
	})
}

// sign_in = role_holders asks HoldsRoleInApp: true with the role, false
// once it is revoked.
func TestHoldsRoleInApp(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := build(t, ctx, q)
		holds, err := repo.HoldsRoleInApp(ctx, w.tenant, w.person, w.app)
		if err != nil || holds {
			t.Fatalf("before any grant: holds = %v, %v", holds, err)
		}
		g := w.grant(t, ctx, q, "Org A")
		if holds, _ = repo.HoldsRoleInApp(ctx, w.tenant, w.person, w.app); !holds {
			t.Fatal("with the role: does not hold")
		}
		_, err = q.Exec(ctx, `UPDATE grants SET revoked_at = now() WHERE id = $1`, g)
		must(t, "revoke", err)
		if holds, _ = repo.HoldsRoleInApp(ctx, w.tenant, w.person, w.app); holds {
			t.Fatal("revoked: still holds")
		}
	})
}
