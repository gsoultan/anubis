//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// The M6 slice moved Authorize, RolesForIdentity, RoleByName and CreateRole
// from sqlc to storm. Every test here checks the MIGRATED repository against
// the database directly, so a scanner or binding mistake shows up as a value
// difference, not a green suite that never crossed the new code.

func sliceRepo() *authzpg.Repository {
	return authzpg.New(database.New(pool))
}

func TestStormSlice_AuthorizeMatchesEngine(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	identity, permission, targets := realDecisionProbe(ctx, t, tenant)
	raw, _ := json.Marshal(targets)

	var direct bool
	if err := pool.QueryRow(ctx, `SELECT authorize($1,$2,$3,$4::jsonb)`,
		identity, tenant, permission, string(raw)).Scan(&direct); err != nil {
		t.Fatal(err)
	}
	got, err := sliceRepo().Authorize(ctx, identity, tenant, permission, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != direct {
		t.Fatalf("repository says %v, the engine says %v — same call, same data", got, direct)
	}
	if !direct {
		t.Log("probe decision is deny; allow path unexercised by this dataset")
	}

	// A permission that cannot exist must come back as a clean deny.
	got, err = sliceRepo().Authorize(ctx, identity, tenant, "storm:slice:never", raw)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("nonexistent permission authorized")
	}
}

func TestStormSlice_RolesForIdentityParity(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)

	var identity string
	if err := pool.QueryRow(ctx,
		`SELECT identity_id FROM grants WHERE tenant_id = $1 AND revoked_at IS NULL LIMIT 1`,
		tenant).Scan(&identity); err != nil {
		t.Skipf("no live grant: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT DISTINCT r.name
		FROM grants g JOIN roles r ON r.id = g.role_id
		WHERE g.identity_id = $1 AND g.tenant_id = $2
		  AND g.revoked_at IS NULL AND g.valid_from <= now()
		  AND (g.valid_until IS NULL OR g.valid_until > now())
		ORDER BY r.name`, identity, tenant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var want []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		want = append(want, n)
	}

	got, err := sliceRepo().RolesForIdentity(ctx, tenant, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("repository: %v, direct SQL: %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("repository: %v, direct SQL: %v", got, want)
		}
	}
}

func TestStormSlice_RoleByNameParity(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)

	var wantID, wantName string
	if err := pool.QueryRow(ctx,
		`SELECT id::text, name FROM roles WHERE tenant_id = $1 ORDER BY name LIMIT 1`,
		tenant).Scan(&wantID, &wantName); err != nil {
		t.Skipf("tenant has no roles: %v", err)
	}

	rec, err := sliceRepo().RoleByName(ctx, tenant, wantName)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != wantID || rec.Name != wantName {
		t.Fatalf("got (%s,%s), want (%s,%s) — the [16]byte→string crossing is wrong",
			rec.ID, rec.Name, wantID, wantName)
	}

	if _, err := sliceRepo().RoleByName(ctx, tenant, "storm-slice-no-such-role"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing role returned %v, want apperr.ErrNotFound", err)
	}
}

// CreateRole and the ambient-transaction path in one test: the write happens
// inside WithinTx (storm sees pgxdrv.Tx, not the pool), is visible to a read
// in the same transaction, and the rollback leaves the database untouched.
func TestStormSlice_CreateRoleInAmbientTx(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()

	const name = "storm-slice-tx-probe"
	sentinel := errors.New("deliberate rollback")
	err := repo.WithinTx(ctx, func(ctx context.Context) error {
		id, err := repo.CreateRole(ctx, tenant, roleRecord(name), "")
		if err != nil {
			return err
		}
		if id == "" {
			t.Fatal("CreateRole returned an empty id")
		}
		rec, err := repo.RoleByName(ctx, tenant, name)
		if err != nil {
			return err
		}
		if rec.ID != id {
			t.Fatalf("read inside the tx sees id %s, create returned %s", rec.ID, id)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithinTx returned %v, want the sentinel", err)
	}
	if _, err := repo.RoleByName(ctx, tenant, name); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("rolled-back role still visible (err=%v)", err)
	}
}

// The budget test measured the engine over bare pgx. This is the same
// methodology through the MIGRATED repository, so the two logs are directly
// comparable and the storm layer's overhead is a subtraction, not a guess.
func TestStormSlice_AuthorizeLatencyBudget(t *testing.T) {
	skipWithoutDB(t)
	if testing.Short() {
		t.Skip("latency budget")
	}
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	identity, permission, targets := realDecisionProbe(ctx, t, tenant)
	raw, _ := json.Marshal(targets)
	repo := sliceRepo()

	assertLatencyBudget(t, "authorize over storm repository", func() error {
		_, err := repo.Authorize(ctx, identity, tenant, permission, raw)
		return err
	}, "the migrated path regressed")
}

func roleRecord(name string) authzdomain.RoleRecord {
	return authzdomain.RoleRecord{Name: name, Description: "storm M6 slice probe"}
}
