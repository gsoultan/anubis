//go:build integration

package memberships

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	"github.com/gsoultan/anubis/internal/platform/database"
)

/*
Memberships (0054), held to authorize() rather than to the rows they write.

Every test builds its own world inside a transaction that is rolled back: a
structure, a permission and two roles. None of them reads existing grants —
the grant and membership families above do, and a freshly bootstrapped
database has none, so in CI they skip. An assertion that skips passes the fix
and the bug alike.
*/

// orgWorld is one structure of places, a permission worth deciding on, and a
// role that confers it (council) beside one that confers nothing (other).
type orgWorld struct {
	tenant, person, axis, perm string
	council, other             string
	node                       map[string]string
}

func buildOrgWorld(t *testing.T, ctx context.Context, q database.Querier) orgWorld {
	t.Helper()
	w := orgWorld{tenant: firstTenant(ctx, t), node: map[string]string{}}
	if err := q.QueryRow(ctx, `
		SELECT i.id::text FROM identities i JOIN realms r ON r.id = i.realm_id
		 WHERE i.tenant_id = $1 AND r.kind = 'internal'
		 ORDER BY i.created_at LIMIT 1`, w.tenant).Scan(&w.person); err != nil {
		t.Fatalf("no internal identity in the first tenant — bootstrap creates one: %v", err)
	}
	sfx := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
	w.axis = "mw_" + sfx
	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("fixture %s: %v", what, err)
		}
	}
	_, err := q.Exec(ctx, `INSERT INTO scope_axes (code, display_name, resolution)
		VALUES ($1, 'Organisation (membership test)', '{"from":"context","key":"org_id"}')`, w.axis)
	must("axis", err)
	top, org, unit := w.axis+"_top", w.axis+"_org", w.axis+"_unit"
	_, err = q.Exec(ctx, `INSERT INTO scope_node_types (code, axis_code, display_name, parent_types) VALUES
		($1, $4, 'Top', '{}'),
		($2, $4, 'Organisation', ARRAY[$1::text, $2::text]),
		($3, $4, 'Unit', ARRAY[$2::text, $3::text])`, top, org, unit, w.axis)
	must("levels", err)
	var root string
	must("root", q.QueryRow(ctx, `SELECT scope_ensure_root($1, $2)::text`, w.tenant, w.axis).Scan(&root))
	add := func(name, typ, parent string) string {
		var id string
		slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
		must(name, q.QueryRow(ctx, `SELECT scope_add_node($1, $2, $3, $4, $5, $6)::text`,
			w.tenant, w.axis, typ, parent, slug, name).Scan(&id))
		w.node[name] = id
		return id
	}
	orgA := add("Organization A", org, root)
	div := add("Division Marketing", unit, orgA)
	dept := add("Department Brand", unit, div)
	add("Work office Jakarta", unit, dept)
	orgB := add("Organization B", org, root)
	add("Work office Surabaya", unit, orgB)

	var app string
	must("application", q.QueryRow(ctx, `INSERT INTO applications (tenant_id, kind, slug, name)
		VALUES ($1, 'service', $2, 'Membership test') RETURNING id::text`, w.tenant, "mw-"+sfx).Scan(&app))
	must("permission", q.QueryRow(ctx, `INSERT INTO permissions (tenant_id, application_id, app_slug, resource, action)
		VALUES ($1, $2, $3, 'campaign', 'approve') RETURNING key`, w.tenant, app, "mw-"+sfx).Scan(&w.perm))
	must("council", q.QueryRow(ctx, `INSERT INTO roles (tenant_id, name) VALUES ($1, $2) RETURNING id::text`,
		w.tenant, "Marketing Council "+sfx).Scan(&w.council))
	must("other", q.QueryRow(ctx, `INSERT INTO roles (tenant_id, name) VALUES ($1, $2) RETURNING id::text`,
		w.tenant, "Nothing Much "+sfx).Scan(&w.other))
	_, err = q.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
		SELECT $1::uuid, id FROM permissions WHERE tenant_id = $2 AND key = $3`, w.council, w.tenant, w.perm)
	must("role permission", err)
	_, err = q.Exec(ctx, `SELECT role_recompute_effective($1::uuid)`, w.council)
	must("recompute", err)
	return w
}

// allows asks authorize() itself, inside the same transaction.
func (w orgWorld) allows(t *testing.T, ctx context.Context, q database.Querier, place string) bool {
	t.Helper()
	id, ok := w.node[place]
	if !ok {
		t.Fatalf("no place %q in the fixture", place)
	}
	var allow bool
	if err := q.QueryRow(ctx, `SELECT authorize($1, $2, $3, jsonb_build_object($4::text, $5::text))`,
		w.person, w.tenant, w.perm, w.axis, id).Scan(&allow); err != nil {
		t.Fatalf("authorize at %s: %v", place, err)
	}
	return allow
}

func (w orgWorld) expect(t *testing.T, ctx context.Context, q database.Querier, when string, want map[string]bool) {
	t.Helper()
	for place, allow := range want {
		if got := w.allows(t, ctx, q, place); got != allow {
			t.Fatalf("%s: authorize at %s = %v, want %v", when, place, got, allow)
		}
	}
}

// refused runs one write that must fail, under a savepoint: a failed
// statement aborts the transaction, and the next check needs it alive.
func refused(t *testing.T, ctx context.Context, q database.Querier, what string, fn func() error) {
	t.Helper()
	if _, err := q.Exec(ctx, "SAVEPOINT membership_guard"); err != nil {
		t.Fatal(err)
	}
	err := fn()
	if _, rerr := q.Exec(ctx, "ROLLBACK TO SAVEPOINT membership_guard"); rerr != nil {
		t.Fatal(rerr)
	}
	if err == nil {
		t.Fatalf("%s: allowed, want refused", what)
	}
}

func inRolledBackTx(t *testing.T, fn func(ctx context.Context, repo *authzpg.Repository, q database.Querier)) {
	t.Helper()
	skipWithoutDB(t)
	db := database.New(pool)
	repo := authzpg.New(db)
	err := repo.WithinTx(context.Background(), func(ctx context.Context) error {
		fn(ctx, repo, db.Conn(ctx))
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}

// The Marketing Council case: a seat on it applies where the person was
// seated and nowhere else, at any depth of the structure.
func TestMembershipAppliesWhereEachMemberIsAssigned(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := buildOrgWorld(t, ctx, q)
		mid, err := repo.CreateMembership(ctx, w.tenant, "Marketing Council "+w.axis, "", w.axis)
		if err != nil {
			t.Fatal(err)
		}
		if m, err := repo.MembershipByID(ctx, w.tenant, mid); err != nil || m.AnchorAxis != w.axis {
			t.Fatalf("MembershipByID = %+v, %v; want anchored on %s", m, err, w.axis)
		}
		if err := repo.ReplaceMembershipEntries(ctx, w.tenant, mid,
			[]membership.MembershipEntryInput{{RoleID: w.council}}); err != nil {
			t.Fatal(err)
		}
		w.expect(t, ctx, q, "before any seat", map[string]bool{"Work office Jakarta": false})

		until := time.Now().Add(72 * time.Hour).Truncate(time.Second)
		jkt, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
			MembershipID: mid, IdentityID: w.person, NodeID: w.node["Work office Jakarta"],
			ValidUntil: &until, Reason: "INC-7 council seat",
		}, w.person)
		if err != nil {
			t.Fatal(err)
		}
		if jkt.AssignmentID == "" || jkt.GrantsCreated != 1 {
			t.Fatalf("seat at the work office = %+v, want one assignment and one grant", jkt)
		}
		w.expect(t, ctx, q, "seated at Work office Jakarta", map[string]bool{
			"Work office Jakarta":  true,
			"Department Brand":     false, // the level above is not the seat
			"Division Marketing":   false,
			"Organization A":       false,
			"Organization B":       false,
			"Work office Surabaya": false,
		})

		// The grant carries the assignment's end date and note.
		var reason string
		var ends time.Time
		if err := q.QueryRow(ctx, `SELECT reason, valid_until FROM grants WHERE via_member_id = $1`,
			jkt.AssignmentID).Scan(&reason, &ends); err != nil {
			t.Fatal(err)
		}
		if reason != "INC-7 council seat" || !ends.Equal(until) {
			t.Fatalf("grant reason %q ends %v, want %q ends %v", reason, ends, "INC-7 council seat", until)
		}

		// A second seat, same membership, another organisation.
		orgB, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
			MembershipID: mid, IdentityID: w.person, NodeID: w.node["Organization B"],
		}, w.person)
		if err != nil {
			t.Fatal(err)
		}
		if orgB.AssignmentID == "" || orgB.AssignmentID == jkt.AssignmentID {
			t.Fatalf("second seat = %+v, want an assignment of its own", orgB)
		}
		w.expect(t, ctx, q, "also seated in Organization B", map[string]bool{
			"Work office Surabaya": true, "Organization B": true,
			"Work office Jakarta": true, "Organization A": false,
		})

		// Repeating a seat creates nothing.
		again, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
			MembershipID: mid, IdentityID: w.person, NodeID: w.node["Work office Jakarta"],
		}, w.person)
		if err != nil || again.AssignmentID != "" || again.GrantsCreated != 0 {
			t.Fatalf("repeated seat = %+v, %v; want nothing created", again, err)
		}

		// Leaving one seat leaves the other exactly as it was.
		n, err := repo.LeaveMembership(ctx, w.tenant, orgB.AssignmentID, w.person, "rotated off")
		if err != nil || n != 1 {
			t.Fatalf("leaving Organization B revoked %d grants (%v), want 1", n, err)
		}
		w.expect(t, ctx, q, "after leaving Organization B", map[string]bool{
			"Work office Surabaya": false, "Work office Jakarta": true,
		})
		var revokeReason string
		if err := q.QueryRow(ctx, `SELECT revoke_reason FROM grants WHERE via_member_id = $1`,
			orgB.AssignmentID).Scan(&revokeReason); err != nil || revokeReason != "rotated off" {
			t.Fatalf("revoke reason %q (%v), want %q", revokeReason, err, "rotated off")
		}

		list, err := repo.ListMembershipAssignments(ctx, w.tenant, membership.MembershipAssignmentFilter{MembershipID: mid})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != jkt.AssignmentID || list[0].NodeName != "Work office Jakarta" ||
			list[0].Exact || list[0].Reason != "INC-7 council seat" || list[0].ValidUntil == nil {
			t.Fatalf("assignments = %+v, want only the Jakarta seat with its end date and note", list)
		}
		// Positive control for the tenant filter: the same membership read as
		// another tenant is simply not there.
		if other, err := repo.ListMembershipAssignments(ctx, "00000000-0000-0000-0000-000000000000",
			membership.MembershipAssignmentFilter{MembershipID: mid}); err != nil || len(other) != 0 {
			t.Fatalf("assignments read as another tenant = %d rows (%v), want none", len(other), err)
		}
		ms, err := repo.ListMemberships(ctx, w.tenant)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.ID == mid && (m.MemberCount != 1 || m.AnchorAxis != w.axis) {
				t.Fatalf("membership listed as %+v, want one member and its anchor", m)
			}
		}
	})
}

// Root cause of the bug this pins: replacing a membership's roles DELETEd its
// entries, and grants point back at an entry with ON DELETE RESTRICT — revoked
// grants included, since they are kept as history — so the first assignment
// froze the membership's contents for good.
func TestMembershipContentsChangeAfterSomeoneJoined(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := buildOrgWorld(t, ctx, q)
		mid, err := repo.CreateMembership(ctx, w.tenant, "Everywhere crew "+w.axis, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.ReplaceMembershipEntries(ctx, w.tenant, mid,
			[]membership.MembershipEntryInput{{RoleID: w.council}}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
			MembershipID: mid, IdentityID: w.person,
		}, w.person); err != nil {
			t.Fatal(err)
		}
		w.expect(t, ctx, q, "holding the council role through the membership", map[string]bool{
			"Organization A": true, "Work office Surabaya": true,
		})

		// Change what the membership gives while somebody is in it.
		if err := repo.ReplaceMembershipEntries(ctx, w.tenant, mid,
			[]membership.MembershipEntryInput{{RoleID: w.other}}); err != nil {
			t.Fatalf("changing a membership somebody holds was refused: %v", err)
		}
		changed, err := repo.ResyncMembership(ctx, mid)
		if err != nil {
			t.Fatal(err)
		}
		if changed != 2 {
			t.Fatalf("resync changed %d grants, want 2 (one revoked, one given)", changed)
		}
		w.expect(t, ctx, q, "after the council role left the membership", map[string]bool{
			"Organization A": false, "Work office Surabaya": false,
		})
		var why string
		if err := q.QueryRow(ctx, `SELECT revoke_reason FROM grants
			WHERE via_membership_id = $1 AND role_id = $2 AND revoked_at IS NOT NULL`, mid, w.council).Scan(&why); err != nil ||
			why == "" {
			t.Fatalf("the grant the membership stopped giving was not revoked with a reason: %q (%v)", why, err)
		}

		// Sending the same contents again changes nothing: an unchanged entry
		// keeps its id, and its members keep their grants.
		before, err := repo.MembershipEntries(ctx, []string{mid})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.ReplaceMembershipEntries(ctx, w.tenant, mid,
			[]membership.MembershipEntryInput{{RoleID: w.other}}); err != nil {
			t.Fatal(err)
		}
		after, err := repo.MembershipEntries(ctx, []string{mid})
		if err != nil {
			t.Fatal(err)
		}
		if len(before) != 1 || len(after) != 1 || before[0].ID != after[0].ID {
			t.Fatalf("unchanged contents re-created the entry: before %+v after %+v", before, after)
		}
		if changed, err := repo.ResyncMembership(ctx, mid); err != nil || changed != 0 {
			t.Fatalf("resync of unchanged contents changed %d grants (%v), want 0", changed, err)
		}

		// And once nobody is in it any more, it can still change.
		if _, err := repo.UnassignMembership(ctx, w.person, mid, w.person, "left"); err != nil {
			t.Fatal(err)
		}
		if err := repo.ReplaceMembershipEntries(ctx, w.tenant, mid,
			[]membership.MembershipEntryInput{{RoleID: w.council}}); err != nil {
			t.Fatalf("changing a membership after its last member left was refused: %v", err)
		}
	})
}

// The rules the schema holds whatever calls it.
func TestMembershipPlaceRules(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := buildOrgWorld(t, ctx, q)
		anchored, err := repo.CreateMembership(ctx, w.tenant, "Anchored "+w.axis, "", w.axis)
		if err != nil {
			t.Fatal(err)
		}
		everywhere, err := repo.CreateMembership(ctx, w.tenant, "Same for all "+w.axis, "", "")
		if err != nil {
			t.Fatal(err)
		}
		refused(t, ctx, q, "a where-assigned membership assigned with no place", func() error {
			_, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
				MembershipID: anchored, IdentityID: w.person}, w.person)
			return err
		})
		refused(t, ctx, q, "a same-for-everyone membership given a place", func() error {
			_, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
				MembershipID: everywhere, IdentityID: w.person, NodeID: w.node["Organization A"]}, w.person)
			return err
		})
		refused(t, ctx, q, "an assignment ending in the past", func() error {
			past := time.Now().Add(-time.Hour)
			_, err := repo.AssignMembership(ctx, membership.MembershipAssignmentInput{
				MembershipID: anchored, IdentityID: w.person, NodeID: w.node["Organization A"],
				ValidUntil: &past}, w.person)
			return err
		})
		refused(t, ctx, q, "a role inside a where-assigned membership naming a place on its own axis", func() error {
			return repo.ReplaceMembershipEntries(ctx, w.tenant, anchored, []membership.MembershipEntryInput{{
				RoleID: w.council,
				Scopes: []grant.GrantScopeInput{{Axis: w.axis, NodeID: w.node["Organization A"], Inherit: true}},
			}})
		})
		refused(t, ctx, q, "changing where a membership applies after it was created", func() error {
			_, err := q.Exec(ctx, `UPDATE memberships SET anchor_axis = NULL WHERE id = $1`, anchored)
			return err
		})
		// A place on some OTHER structure, written straight into the table:
		// the trigger is the floor under writes that skip membership_assign.
		// The second structure is built here, so this cannot pass on nothing.
		other := w.axis + "_x"
		if _, err := q.Exec(ctx, `INSERT INTO scope_axes (code, display_name) VALUES ($1, 'Other (membership test)')`, other); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Exec(ctx, `INSERT INTO scope_node_types (code, axis_code, display_name, parent_types)
			VALUES ($1, $2, 'Top', '{}')`, other+"_top", other); err != nil {
			t.Fatal(err)
		}
		var elsewhere string
		if err := q.QueryRow(ctx, `SELECT scope_ensure_root($1, $2)::text`, w.tenant, other).Scan(&elsewhere); err != nil {
			t.Fatal(err)
		}
		refused(t, ctx, q, "an assignment whose place is on another structure", func() error {
			_, err := q.Exec(ctx, `INSERT INTO membership_members
				(membership_id, identity_id, tenant_id, assigned_by, axis_code, scope_node_id)
				VALUES ($1, $2, $3, $2, $4, $5)`, anchored, w.person, w.tenant, other, elsewhere)
			return err
		})
	})
}

// Asking about no place at all is a question with an answer, for anybody.
//
// A caller that sends no targets marshals a nil map, which is the JSON literal
// null — and authorize() iterates its targets with jsonb_each_text, which
// refuses anything but an object. The statement only reaches that for a person
// holding a place-limited grant, so it stayed hidden behind people who held
// none: Explain and the strict dry run answered them, and failed everybody
// else with an internal error.
//
// Root cause, in one sentence: the "no targets means {}" guard lived in one
// interactor instead of at the SQL boundary all three decision calls share.
func TestNoTargetsIsADecisionForSomeoneWithPlaceLimitedAccess(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, repo *authzpg.Repository, q database.Querier) {
		w := buildOrgWorld(t, ctx, q)
		var place string
		for _, id := range w.node {
			place = id
			break
		}
		if _, err := repo.CreateGrant(ctx, grant.GrantCreate{
			TenantID: w.tenant, IdentityID: w.person, RoleID: w.council, GrantedBy: w.person,
			Scopes: []grant.GrantScopeInput{{Axis: w.axis, NodeID: place, Inherit: true}},
		}); err != nil {
			t.Fatalf("a place-limited grant: %v", err)
		}
		for _, targets := range [][]byte{nil, []byte("null"), []byte("{}")} {
			if _, err := repo.Authorize(ctx, w.person, w.tenant, w.perm, targets); err != nil {
				t.Fatalf("authorize with targets %q: %v", targets, err)
			}
			detail, err := repo.AuthorizeExplain(ctx, w.person, w.tenant, w.perm, targets)
			if err != nil || detail == "" {
				t.Fatalf("explain with targets %q: %q, %v", targets, detail, err)
			}
			if _, err := repo.AuthorizeStrictSim(ctx, w.person, w.tenant, w.perm, targets, w.axis); err != nil {
				t.Fatalf("strict dry run with targets %q: %v", targets, err)
			}
		}
	})
}
