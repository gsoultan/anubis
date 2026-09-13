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
