//go:build integration

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	_ "github.com/jackc/pgx/v5/stdlib"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

/*
An admin RPC that takes an id has two jobs, and the guard only does one of
them. `Require` answers "may this caller read identities?" — it never answers
"does THIS id belong to the caller's tenant?". That second question is the
query's, and it can only ask it if the interactor passes the tenant down.

Wherever an interactor writes `if _, err := u.guard.Require(...)` it has
thrown the principal away, so the tenant cannot reach the query, so the id is
taken on trust. That discard is the tell, and it found every case here.

This test plants a whole second tenant and hands its ids to an operator who
administers a different one. Everything below leaked before the fix.
*/

/*
Each test below carries a POSITIVE control, and it is not ceremony.

Adding the tenant filter to ListCredentials initially left the repository
passing an empty tenant id. Every isolation assertion still passed — an empty
uuid matches nothing, which from outside is indistinguishable from "correctly
refused". A security test that cannot tell a working filter from a broken
query is a test that will wave the regression through. So each one also proves
the operator can still read their OWN tenant through the same call.
*/

// ownTenant is the tenant the e2e operator administers, which is the one every
// request carries in X-Anubis-Tenant.
func ownTenant(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM tenants WHERE slug = $1`, tenant).Scan(&id); err != nil {
		t.Fatalf("find own tenant %q: %v", tenant, err)
	}
	return id
}

// neighbour is a complete foreign tenant: a realm, a person with a credential,
// a role with an effective permission, and a two-node scope tree. Enough for
// each read path to have something of someone else's to hand over.
type neighbour struct {
	tenantID   string
	identityID string
	roleID     string
	roleName   string
	childNode  string
	rootName   string
	credLabel  string
}

func plantNeighbour(t *testing.T, ctx context.Context, db *sql.DB) neighbour {
	t.Helper()
	n := neighbour{
		roleName:  fmt.Sprintf("neighbour-role-%d", time.Now().UnixNano()),
		rootName:  fmt.Sprintf("Neighbour HQ %d", time.Now().UnixNano()),
		credLabel: fmt.Sprintf("neighbour-laptop-%d", time.Now().UnixNano()),
	}
	suffix := time.Now().UnixNano()

	if err := db.QueryRowContext(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Neighbour %d", suffix), fmt.Sprintf("neighbour-%d", suffix),
	).Scan(&n.tenantID); err != nil {
		t.Fatalf("plant tenant: %v", err)
	}
	// applications, identities and scope_nodes each RESTRICT the tenant
	// delete, so the fixture tears down in dependency order. Reported rather
	// than swallowed: a tenant left behind is a realm every later snapshot
	// rebuild loads forever.
	t.Cleanup(func() {
		bg := context.Background()
		for _, tbl := range []string{
			"scope_nodes", "credentials", "identities",
			"permissions", "applications", "roles", "realms",
		} {
			if _, err := db.ExecContext(bg,
				"DELETE FROM "+tbl+" WHERE tenant_id = $1", n.tenantID); err != nil {
				t.Errorf("planted %s not removed: %v", tbl, err)
				return
			}
		}
		if _, err := db.ExecContext(bg, `DELETE FROM tenants WHERE id = $1`, n.tenantID); err != nil {
			t.Errorf("planted tenant %s not removed: %v", n.tenantID, err)
		}
	})

	var realmID string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO realms (tenant_id, code, kind, display_name)
		 VALUES ($1, 'internal', 'internal', 'Neighbour staff') RETURNING id`,
		n.tenantID).Scan(&realmID); err != nil {
		t.Fatalf("plant realm: %v", err)
	}

	// A person and the credential that says how they sign in.
	if err := db.QueryRowContext(ctx,
		`INSERT INTO identities (tenant_id, realm_id, username, email)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		n.tenantID, realmID, fmt.Sprintf("neighbour-user-%d", suffix),
		fmt.Sprintf("neighbour%d@elsewhere.test", suffix),
	).Scan(&n.identityID); err != nil {
		t.Fatalf("plant identity: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO credentials (tenant_id, identity_id, kind, label)
		 VALUES ($1, $2, 'password', $3)`,
		n.tenantID, n.identityID, n.credLabel); err != nil {
		t.Fatalf("plant credential: %v", err)
	}

	// A role that actually confers something: an empty role would let the
	// read path pass by returning nothing it had.
	var appID, permID string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO applications (tenant_id, kind, slug, name)
		 VALUES ($1, 'service', $2, 'Neighbour app') RETURNING id`,
		n.tenantID, fmt.Sprintf("neighbour-app-%d", suffix)).Scan(&appID); err != nil {
		t.Fatalf("plant application: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO permissions (tenant_id, application_id, app_slug, resource, action)
		 VALUES ($1, $2, $3, 'merger', 'read') RETURNING id`,
		n.tenantID, appID, fmt.Sprintf("neighbour-app-%d", suffix)).Scan(&permID); err != nil {
		t.Fatalf("plant permission: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO roles (tenant_id, name) VALUES ($1, $2) RETURNING id`,
		n.tenantID, n.roleName).Scan(&n.roleID); err != nil {
		t.Fatalf("plant role: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO role_permissions_effective (role_id, permission_id, via_role_id)
		 VALUES ($1, $2, $1)`, n.roleID, permID); err != nil {
		t.Fatalf("plant effective permission: %v", err)
	}

	// Two nodes on an existing axis, plus the closure rows the ancestor walk
	// reads. The ROOT's name is the thing that must not come back.
	// The node-type hierarchy is enforced by a check constraint, so the pair
	// has to be legal: a root type that takes no parent, and a child type
	// that names it. Picking the axis's first type for both is rejected with
	// `a "org" may not sit under a "org"`.
	var axis, rootType, childType string
	if err := db.QueryRowContext(ctx,
		`SELECT root.axis_code, root.code, child.code
		   FROM scope_node_types root
		   JOIN scope_node_types child
		     ON child.axis_code = root.axis_code
		    AND root.code = ANY (child.parent_types)
		  WHERE cardinality(root.parent_types) = 0
		  ORDER BY root.axis_code
		  LIMIT 1`).Scan(&axis, &rootType, &childType); err != nil {
		t.Fatalf("find a legal root/child node-type pair: %v", err)
	}
	var rootNode string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO scope_nodes (tenant_id, axis_code, node_type, slug, name, is_axis_root)
		 VALUES ($1, $2, $3, $4, $5, true) RETURNING id`,
		n.tenantID, axis, rootType, fmt.Sprintf("neighbour-root-%d", suffix), n.rootName,
	).Scan(&rootNode); err != nil {
		t.Fatalf("plant root node: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO scope_nodes (tenant_id, axis_code, node_type, slug, name, parent_id)
		 VALUES ($1, $2, $3, $4, 'Neighbour branch', $5) RETURNING id`,
		n.tenantID, axis, childType, fmt.Sprintf("neighbour-child-%d", suffix), rootNode,
	).Scan(&n.childNode); err != nil {
		t.Fatalf("plant child node: %v", err)
	}
	// The trigger may already maintain closure; ON CONFLICT keeps the fixture
	// correct either way rather than depending on which.
	for _, row := range [][]any{
		{rootNode, rootNode, 0}, {n.childNode, n.childNode, 0}, {rootNode, n.childNode, 1},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, row...); err != nil {
			t.Fatalf("plant closure: %v", err)
		}
	}
	return n
}

// ScopeAncestors took the node id straight from the request. The interactor
// above it discarded the principal, so the tenant never reached a query that
// filters on descendant_id alone — and the answer is the names of the boxes
// above a node, which is the shape of somebody else's organisation.
func TestScopeAncestorsRefuseAnotherTenantsNode(t *testing.T) {
	requireServer(t)
	db, ctx, token := isolationFixture(t)
	n := plantNeighbour(t, ctx, db)

	scope := anubisv1connect.NewScopeAdminServiceClient(http.DefaultClient, baseURL)
	resp, err := scope.ScopeAncestors(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ScopeAncestorsRequest{Id: n.childNode}), token))
	if err != nil {
		return // refused outright is a correct answer
	}
	for _, a := range resp.Msg.GetAncestors() {
		t.Fatalf("read another tenant's scope tree: ancestor %q (%s)",
			a.GetNode().GetName(), a.GetNode().GetId())
	}

	// Positive control: a node of the operator's own must still resolve.
	var ownNode string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM scope_nodes
		  WHERE tenant_id = $1 AND parent_id IS NOT NULL LIMIT 1`,
		ownTenant(t, ctx, db)).Scan(&ownNode); err != nil {
		t.Skipf("own tenant has no child scope node to check against: %v", err)
	}
	own, err := scope.ScopeAncestors(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ScopeAncestorsRequest{Id: ownNode}), token))
	if err != nil {
		t.Fatalf("own tenant's node refused: %v", err)
	}
	if len(own.Msg.GetAncestors()) == 0 {
		t.Fatal("own tenant's node returned no ancestors — the filter is not " +
			"scoping, it is matching nothing")
	}
}

// Rename, restore and archive name a node by id, and the node ids of every
// tenant share one table. Each must find the node inside the caller's tenant
// or not at all.
func TestScopeNodeEditsRefuseAnotherTenantsNode(t *testing.T) {
	requireServer(t)
	db, ctx, token := isolationFixture(t)
	n := plantNeighbour(t, ctx, db)
	scope := anubisv1connect.NewScopeAdminServiceClient(http.DefaultClient, baseURL)
	state := func(id string) (name, status string) {
		t.Helper()
		if err := db.QueryRowContext(ctx, `SELECT name, status FROM scope_nodes WHERE id = $1`, id).
			Scan(&name, &status); err != nil {
			t.Fatalf("read node: %v", err)
		}
		return name, status
	}
	before, _ := state(n.childNode)

	if _, err := scope.RenameScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.RenameScopeNodeRequest{NodeId: n.childNode, Name: "renamed by a neighbour"}), token)); err == nil {
		t.Fatal("renamed another tenant's node")
	}
	if name, _ := state(n.childNode); name != before {
		t.Fatalf("refused, yet the node is now called %q", name)
	}
	if _, err := scope.ArchiveScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ArchiveScopeNodeRequest{NodeId: n.childNode}), token)); err == nil {
		t.Fatal("archived another tenant's node")
	}
	if _, err := db.ExecContext(ctx, `UPDATE scope_nodes SET status = 'archived' WHERE id = $1`, n.childNode); err != nil {
		t.Fatal(err)
	}
	if _, err := scope.RestoreScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.RestoreScopeNodeRequest{NodeId: n.childNode}), token)); err == nil {
		t.Fatal("restored another tenant's node")
	}
	if _, status := state(n.childNode); status != "archived" {
		t.Fatalf("refused, yet the node is now %s", status)
	}

	// Positive control: the same three calls on a node of the operator's own,
	// each undone. A check that refuses everything passes everything above.
	var own string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM scope_nodes WHERE tenant_id = $1 AND parent_id IS NOT NULL
		    AND status = 'active' ORDER BY created_at LIMIT 1`,
		ownTenant(t, ctx, db)).Scan(&own); err != nil {
		t.Fatalf("own tenant has no child scope node to check against: %v", err)
	}
	name, _ := state(own)
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(),
			`UPDATE scope_nodes SET name = $2, status = 'active' WHERE id = $1`, own, name); err != nil {
			t.Errorf("own node not put back: %v", err)
		}
	})
	if _, err := scope.RenameScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.RenameScopeNodeRequest{NodeId: own, Name: name + " (e2e)"}), token)); err != nil {
		t.Fatalf("own node: rename refused: %v", err)
	}
	if got, _ := state(own); got != name+" (e2e)" {
		t.Fatalf("own node: rename accepted but the name is %q", got)
	}
	if _, err := scope.ArchiveScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ArchiveScopeNodeRequest{NodeId: own}), token)); err != nil {
		t.Fatalf("own node: archive refused: %v", err)
	}
	if _, err := scope.RenameScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.RenameScopeNodeRequest{NodeId: own, Name: name}), token)); err == nil {
		t.Fatal("own node: renamed while archived — a restore hidden inside a rename")
	}
	if _, err := scope.RestoreScopeNode(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.RestoreScopeNodeRequest{NodeId: own}), token)); err != nil {
		t.Fatalf("own node: restore refused: %v", err)
	}
	if _, status := state(own); status != "active" {
		t.Fatalf("own node: restore accepted but the node is %s", status)
	}
}

// ListCredentials took the identity id straight from the request. The
// credentials table carries tenant_id and the query even SELECTs it — it just
// never filtered on it, so the answer was another tenant's inventory of how
// their people sign in.
func TestListCredentialsRefuseAnotherTenantsIdentity(t *testing.T) {
	requireServer(t)
	db, ctx, token := isolationFixture(t)
	n := plantNeighbour(t, ctx, db)

	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	resp, err := idAdmin.ListCredentials(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListCredentialsRequest{IdentityId: n.identityID}), token))
	if err != nil {
		return
	}
	for _, c := range resp.Msg.GetCredentials() {
		t.Fatalf("read another tenant's credential: kind %q label %q",
			c.GetKind(), c.GetLabel())
	}

	// Positive control: this is the assertion that caught an empty tenant id
	// being passed where the caller's belonged.
	var ownIdentity string
	if err := db.QueryRowContext(ctx,
		`SELECT identity_id FROM credentials WHERE tenant_id = $1 LIMIT 1`,
		ownTenant(t, ctx, db)).Scan(&ownIdentity); err != nil {
		t.Skipf("own tenant has no credential to check against: %v", err)
	}
	own, err := idAdmin.ListCredentials(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListCredentialsRequest{IdentityId: ownIdentity}), token))
	if err != nil {
		t.Fatalf("own tenant's identity refused: %v", err)
	}
	if len(own.Msg.GetCredentials()) == 0 {
		t.Fatal("own tenant's identity returned no credentials — the filter is " +
			"not scoping, it is matching nothing")
	}
}

// GetRoleEffective took the role id straight from the request. Two
// declarations below it in the same file, ListRolesUsingPattern filters
// `r.tenant_id = $1` — the habit was there, this query just did not have it.
func TestRoleEffectiveRefusesAnotherTenantsRole(t *testing.T) {
	requireServer(t)
	db, ctx, token := isolationFixture(t)
	n := plantNeighbour(t, ctx, db)

	authz := anubisv1connect.NewAuthzAdminServiceClient(http.DefaultClient, baseURL)
	resp, err := authz.GetRoleEffective(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.GetRoleEffectiveRequest{RoleId: n.roleID}), token))
	if err != nil {
		return
	}
	for _, p := range resp.Msg.GetPermissions() {
		t.Fatalf("read another tenant's role: permission %q via %q",
			p.GetPermissionKey(), p.GetViaRole())
	}

	// Positive control.
	var ownRole string
	if err := db.QueryRowContext(ctx,
		`SELECT r.id FROM roles r
		   JOIN role_permissions_effective rpe ON rpe.role_id = r.id
		  WHERE r.tenant_id = $1 LIMIT 1`,
		ownTenant(t, ctx, db)).Scan(&ownRole); err != nil {
		t.Skipf("own tenant has no role with effective permissions: %v", err)
	}
	own, err := authz.GetRoleEffective(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.GetRoleEffectiveRequest{RoleId: ownRole}), token))
	if err != nil {
		t.Fatalf("own tenant's role refused: %v", err)
	}
	if len(own.Msg.GetPermissions()) == 0 {
		t.Fatal("own tenant's role returned no permissions — the filter is not " +
			"scoping, it is matching nothing")
	}
}

func isolationFixture(t *testing.T) (*sql.DB, context.Context, string) {
	t.Helper()
	dbURL := os.Getenv("ANUBIS_DB_URL")
	if dbURL == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Registered before plantNeighbour's row cleanup, so LIFO closes the pool
	// last — a deferred Close would shut it before the delete could run.
	t.Cleanup(func() { _ = db.Close() })
	return db, context.Background(), platformLogin(t)
}

/*
Membership writes took the membership id straight from the request.

AssignMembership, UnassignMembership, ResyncMembership and SetMembershipEntries
each asked the guard whether the caller may administer memberships, then threw
the principal away — the tell this file's neighbour describes. The foreign keys
only checked that a membership and a person agreed with EACH OTHER, so an
operator of one tenant could put a second tenant's person into that tenant's
membership and hand them its roles.

Root cause, in one sentence: the tenant never reached the membership lookup,
so any membership id was taken on trust.
*/
func TestMembershipWritesRefuseAnotherTenantsMembership(t *testing.T) {
	requireServer(t)
	db, ctx, token := isolationFixture(t)
	n := plantNeighbour(t, ctx, db)

	// Theirs: a membership giving their role, with their person already in
	// it — something for every call below to change, and for the list to leak.
	suffix := time.Now().UnixNano()
	var theirs string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO memberships (tenant_id, name) VALUES ($1, $2) RETURNING id`,
		n.tenantID, fmt.Sprintf("neighbour-membership-%d", suffix)).Scan(&theirs); err != nil {
		t.Fatalf("plant membership: %v", err)
	}
	// Registered after plantNeighbour's cleanup, so it runs first: their
	// grants and memberships hold their roles and people in place.
	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM grant_scopes WHERE tenant_id = $1`,
			`DELETE FROM grants WHERE tenant_id = $1`,
			`DELETE FROM memberships WHERE tenant_id = $1`,
		} {
			if _, err := db.ExecContext(bg, stmt, n.tenantID); err != nil {
				t.Errorf("planted membership rows not removed: %v", err)
			}
		}
	})
	if _, err := db.ExecContext(ctx,
		`INSERT INTO membership_entries (membership_id, tenant_id, role_id) VALUES ($1, $2, $3)`,
		theirs, n.tenantID, n.roleID); err != nil {
		t.Fatalf("plant membership entry: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO membership_members (membership_id, identity_id, tenant_id, assigned_by)
		 VALUES ($1, $2, $3, $2)`, theirs, n.identityID, n.tenantID); err != nil {
		t.Fatalf("plant membership member: %v", err)
	}
	// And a second person of theirs who is NOT in it yet: adding them is a
	// write that creates something, which is what makes the hole visible —
	// adding somebody already in a membership changes nothing either way.
	var newcomer string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO identities (tenant_id, realm_id, username)
		 SELECT tenant_id, realm_id, $2 FROM identities WHERE id = $1 RETURNING id`,
		n.identityID, fmt.Sprintf("neighbour-newcomer-%d", suffix)).Scan(&newcomer); err != nil {
		t.Fatalf("plant a second person: %v", err)
	}
	count := func(query string) int {
		t.Helper()
		var c int
		if err := db.QueryRowContext(ctx, query, theirs).Scan(&c); err != nil {
			t.Fatalf("count: %v", err)
		}
		return c
	}
	grantsOf := `SELECT count(*) FROM grants WHERE via_membership_id = $1`

	authz := anubisv1connect.NewAuthzAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := authz.AssignMembership(ctx, operatorBearer(connect.NewRequest(&anubisv1.AssignMembershipRequest{
		MembershipId: theirs, IdentityId: newcomer,
	}), token)); err == nil {
		t.Fatalf("added another tenant's person to that tenant's membership (%d grants now)", count(grantsOf))
	}
	if got := count(grantsOf); got != 0 {
		t.Fatalf("refused, yet %d grants were written through another tenant's membership", got)
	}
	if _, err := authz.ResyncMembership(ctx, operatorBearer(connect.NewRequest(&anubisv1.ResyncMembershipRequest{
		MembershipId: theirs,
	}), token)); err == nil {
		t.Fatal("resynced another tenant's membership")
	}
	if got := count(grantsOf); got != 0 {
		t.Fatalf("resync was refused, yet it wrote %d grants", got)
	}
	if _, err := authz.SetMembershipEntries(ctx, operatorBearer(connect.NewRequest(&anubisv1.SetMembershipEntriesRequest{
		MembershipId: theirs,
	}), token)); err == nil {
		t.Fatal("replaced the contents of another tenant's membership")
	}
	if got := count(`SELECT count(*) FROM membership_entries WHERE membership_id = $1 AND retired_at IS NULL`); got != 1 {
		t.Fatalf("another tenant's membership now has %d roles, want its 1 untouched", got)
	}
	if _, err := authz.UnassignMembership(ctx, operatorBearer(connect.NewRequest(&anubisv1.UnassignMembershipRequest{
		MembershipId: theirs, IdentityId: n.identityID,
	}), token)); err == nil {
		t.Fatal("removed a person from another tenant's membership")
	}
	if got := count(`SELECT count(*) FROM membership_members WHERE membership_id = $1 AND removed_at IS NULL`); got != 1 {
		t.Fatalf("another tenant's membership now has %d members, want its 1 untouched", got)
	}
	list, err := authz.ListMembershipAssignments(ctx, operatorBearer(connect.NewRequest(&anubisv1.ListMembershipAssignmentsRequest{
		MembershipId: theirs,
	}), token))
	if err == nil {
		for _, a := range list.Msg.GetAssignments() {
			t.Fatalf("read another tenant's membership roster: %s (%s)", a.GetUsername(), a.GetIdentityId())
		}
	}

	// Positive control: the operator's own tenant, through the same calls. A
	// check that refuses everything would pass every assertion above.
	own := ownTenant(t, ctx, db)
	var person string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM identities WHERE tenant_id = $1 ORDER BY created_at LIMIT 1`, own).Scan(&person); err != nil {
		t.Fatalf("own tenant has nobody to add: %v", err)
	}
	created, err := authz.CreateMembership(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateMembershipRequest{
		Name: fmt.Sprintf("own-membership-%d", suffix),
	}), token))
	if err != nil {
		t.Fatalf("own tenant: create membership: %v", err)
	}
	mine := created.Msg.GetMembership().GetId()
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM memberships WHERE id = $1`, mine); err != nil {
			t.Errorf("own membership not removed: %v", err)
		}
	})
	assigned, err := authz.AssignMembership(ctx, operatorBearer(connect.NewRequest(&anubisv1.AssignMembershipRequest{
		MembershipId: mine, IdentityId: person,
	}), token))
	if err != nil {
		t.Fatalf("own tenant: assign refused: %v", err)
	}
	if assigned.Msg.GetAssignmentId() == "" {
		t.Fatal("own tenant: assign created no assignment")
	}
	roster, err := authz.ListMembershipAssignments(ctx, operatorBearer(connect.NewRequest(&anubisv1.ListMembershipAssignmentsRequest{
		MembershipId: mine,
	}), token))
	if err != nil || len(roster.Msg.GetAssignments()) != 1 {
		t.Fatalf("own tenant: roster = %d (%v), want the 1 person just added", len(roster.Msg.GetAssignments()), err)
	}
	if _, err := authz.UnassignMembership(ctx, operatorBearer(connect.NewRequest(&anubisv1.UnassignMembershipRequest{
		AssignmentId: assigned.Msg.GetAssignmentId(), Reason: "e2e",
	}), token)); err != nil {
		t.Fatalf("own tenant: removing the assignment refused: %v", err)
	}
}
