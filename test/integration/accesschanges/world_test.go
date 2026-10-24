//go:build integration

package accesschanges

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// world is one kind of place with three organisations under its top, a
// permission, and a role that gives it.
type world struct {
	tenant, person, axis, perm, app, role, root string
	node                                        map[string]string
	orgType                                     string
}

func inRolledBackTx(t *testing.T, fn func(ctx context.Context, repo *authzpg.Repository, q database.Querier)) {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
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

func must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("fixture %s: %v", what, err)
	}
}

func build(t *testing.T, ctx context.Context, q database.Querier) world {
	t.Helper()
	w := world{node: map[string]string{}}
	must(t, "tenant", q.QueryRow(ctx, `SELECT id::text FROM tenants ORDER BY created_at LIMIT 1`).Scan(&w.tenant))
	must(t, "person", q.QueryRow(ctx, `
		SELECT i.id::text FROM identities i JOIN realms r ON r.id = i.realm_id
		 WHERE i.tenant_id = $1 AND r.kind = 'internal'
		 ORDER BY i.created_at LIMIT 1`, w.tenant).Scan(&w.person))
	sfx := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
	w.axis = "ac_" + sfx
	_, err := q.Exec(ctx, `INSERT INTO scope_axes (tenant_id, code, display_name, resolution)
		VALUES ($2, $1, 'Organisation (bulk test)', '{"from":"context","key":"org_id"}')`, w.axis, w.tenant)
	must(t, "axis", err)
	top := w.axis + "_top"
	w.orgType = w.axis + "_org"
	_, err = q.Exec(ctx, `INSERT INTO scope_node_types (tenant_id, code, axis_code, display_name, parent_types) VALUES
		($4, $1, $3, 'Top', '{}'),
		($4, $2, $3, 'Organisation', ARRAY[$1::text])`, top, w.orgType, w.axis, w.tenant)
	must(t, "levels", err)
	must(t, "root", q.QueryRow(ctx, `SELECT scope_ensure_root($1, $2)::text`, w.tenant, w.axis).Scan(&w.root))
	for _, n := range []string{"Org A", "Org B", "Org C"} {
		w.add(t, ctx, q, n)
	}
	w.app = "ac-" + sfx
	var appID string
	must(t, "application", q.QueryRow(ctx, `INSERT INTO applications (tenant_id, kind, slug, name)
		VALUES ($1, 'service', $2, 'Bulk test') RETURNING id::text`, w.tenant, w.app).Scan(&appID))
	w.app = appID
	must(t, "permission", q.QueryRow(ctx, `INSERT INTO permissions (tenant_id, application_id, app_slug, resource, action)
		VALUES ($1, $2, $3, 'invoice', 'read') RETURNING key`, w.tenant, appID, "ac-"+sfx).Scan(&w.perm))
	must(t, "role", q.QueryRow(ctx, `INSERT INTO roles (tenant_id, name) VALUES ($1, $2) RETURNING id::text`,
		w.tenant, "Invoice reader "+sfx).Scan(&w.role))
	_, err = q.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
		SELECT $1::uuid, id FROM permissions WHERE tenant_id = $2 AND key = $3`, w.role, w.tenant, w.perm)
	must(t, "role permission", err)
	_, err = q.Exec(ctx, `SELECT role_recompute_effective($1::uuid)`, w.role)
	must(t, "recompute", err)
	return w
}

func (w *world) add(t *testing.T, ctx context.Context, q database.Querier, name string) string {
	t.Helper()
	var id string
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	must(t, name, q.QueryRow(ctx, `SELECT scope_add_node($1, $2, $3, $4, $5, $6)::text`,
		w.tenant, w.axis, w.orgType, w.root, slug, name).Scan(&id))
	w.node[name] = id
	return id
}

// grant gives the person the role, picked at the named places.
func (w world) grant(t *testing.T, ctx context.Context, q database.Querier, places ...string) string {
	t.Helper()
	var id string
	must(t, "grant", q.QueryRow(ctx, `INSERT INTO grants (tenant_id, identity_id, role_id, granted_by)
		VALUES ($1, $2, $3, $2) RETURNING id::text`, w.tenant, w.person, w.role).Scan(&id))
	for _, p := range places {
		_, err := q.Exec(ctx, `INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit)
			VALUES ($1, $2, $3, $4, true)`, id, w.tenant, w.axis, w.node[p])
		must(t, "pick "+p, err)
	}
	return id
}

func (w world) allows(t *testing.T, ctx context.Context, q database.Querier, place string) bool {
	t.Helper()
	var allow bool
	if err := q.QueryRow(ctx, `SELECT authorize($1, $2, $3, jsonb_build_object($4::text, $5::text))`,
		w.person, w.tenant, w.perm, w.axis, w.node[place]).Scan(&allow); err != nil {
		t.Fatalf("authorize at %s: %v", place, err)
	}
	return allow
}

// commitChecks fires the deferred guards now. The transaction is rolled
// back at the end, so without this a grant the exclusion guard would refuse
// at COMMIT would sail through every test.
func commitChecks(t *testing.T, ctx context.Context, q database.Querier) {
	t.Helper()
	if _, err := q.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a deferred guard refused the change: %v", err)
	}
	if _, err := q.Exec(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		t.Fatal(err)
	}
}
