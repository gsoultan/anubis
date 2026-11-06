//go:build integration

// Package stepup holds the online decision's step-up gate to the permission
// rows a tenant actually writes, on a real database.
//
//	ANUBIS_DB_URL=... go test -tags integration ./test/integration/stepup/
package stepup

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

// world is one probe tenant holding a person, an application and a role that
// gives one permission, granted to that person everywhere.
type world struct {
	tenant, identity, perm string
}

var w world

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
	ctx := context.Background()
	slug := fmt.Sprintf("zzstep%d", time.Now().UnixNano()%1_000_000_000)
	if err := build(ctx, slug); err != nil {
		cleanup(ctx)
		panic("build probe tenant: " + err.Error())
	}
	code := m.Run()
	cleanup(ctx)
	p.Close()
	os.Exit(code)
}

func build(ctx context.Context, slug string) error {
	var realm, app, role, perm string
	steps := []struct {
		q    string
		args []any
		out  *string
	}{
		{`INSERT INTO tenants (slug, name) VALUES ($1, 'Step-up probe') RETURNING id::text`, []any{slug}, &w.tenant},
		{`INSERT INTO realms (tenant_id, code, kind, display_name) VALUES ($1::uuid, 'internal', 'internal', 'Internal') RETURNING id::text`, nil, &realm},
		{`INSERT INTO identities (tenant_id, realm_id, username) VALUES ($1::uuid, $2::uuid, $3) RETURNING id::text`, nil, &w.identity},
		{`INSERT INTO applications (tenant_id, kind, slug, name) VALUES ($1::uuid, 'service', $2, $2) RETURNING id::text`, nil, &app},
		{`INSERT INTO permissions (tenant_id, application_id, app_slug, resource, action, max_auth_age)
		  VALUES ($1::uuid, $2::uuid, $3, 'payroll', 'approve', '1 day') RETURNING id::text`, nil, &perm},
		{`SELECT key FROM permissions WHERE id = $1::uuid`, nil, &w.perm},
		{`INSERT INTO roles (tenant_id, application_id, name, allowed_realm_kinds) VALUES ($1::uuid, $2::uuid, 'approver', '{internal}') RETURNING id::text`, nil, &role},
		{`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1::uuid, $2::uuid) RETURNING role_id::text`, nil, new(string)},
		{`SELECT (role_recompute_effective($1::uuid) IS NULL)::text`, nil, new(string)},
		{`INSERT INTO grants (tenant_id, identity_id, role_id, granted_by) VALUES ($1::uuid, $2::uuid, $3::uuid, $2::uuid) RETURNING id::text`, nil, new(string)},
	}
	// Arguments are filled in once the ids they depend on exist.
	argsFor := func(i int) []any {
		switch i {
		case 0:
			return steps[0].args
		case 1:
			return []any{w.tenant}
		case 2:
			return []any{w.tenant, realm, slug}
		case 3:
			return []any{w.tenant, slug}
		case 4:
			return []any{w.tenant, app, slug}
		case 5:
			return []any{perm}
		case 6:
			return []any{w.tenant, app}
		case 7:
			return []any{role, perm}
		case 8:
			return []any{role}
		default:
			return []any{w.tenant, w.identity, role}
		}
	}
	for i, s := range steps {
		if err := pool.QueryRow(ctx, s.q, argsFor(i)...).Scan(s.out); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
	}
	return nil
}

func cleanup(ctx context.Context) {
	if w.tenant == "" {
		return
	}
	for _, q := range []string{
		`DELETE FROM grants WHERE tenant_id = $1`,
		`DELETE FROM roles WHERE tenant_id = $1`,
		`DELETE FROM permissions WHERE tenant_id = $1`,
		`DELETE FROM applications WHERE tenant_id = $1`,
		`DELETE FROM identities WHERE tenant_id = $1`,
		`DELETE FROM realms WHERE tenant_id = $1`,
		`DELETE FROM tenants WHERE id = $1`,
	} {
		_, _ = pool.Exec(ctx, q, w.tenant)
	}
}
