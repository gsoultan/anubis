//go:build integration

package accesschanges

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancypg "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres"
)

// A group of companies: an operator assigned to the parent runs the child,
// one assigned to the child does not run the parent, and the tree refuses a
// loop in words an operator can act on.
func TestAGroupOfCompanies(t *testing.T) {
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	db := database.New(pool)
	err := db.WithinTx(context.Background(), func(ctx context.Context) error {
		ten := tenancypg.New(db)
		ctl := controlpg.New(db)
		q := db.Conn(ctx)
		sfx := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
		group, err := ten.CreateTenant(ctx, "grp-"+sfx, "Group", "")
		must(t, "group", err)
		child, err := ten.CreateTenant(ctx, "sub-"+sfx, "Subsidiary", group.ID)
		must(t, "child", err)
		if child.ParentID != group.ID {
			t.Fatalf("child parent = %q, want %q", child.ParentID, group.ID)
		}
		grand, err := ten.CreateTenant(ctx, "br-"+sfx, "Branch", child.ID)
		must(t, "grandchild", err)

		if _, err := q.Exec(ctx, "SAVEPOINT loop"); err != nil {
			t.Fatal(err)
		}
		err = ten.MoveTenant(ctx, group.ID, grand.ID)
		if _, rerr := q.Exec(ctx, "ROLLBACK TO SAVEPOINT loop"); rerr != nil {
			t.Fatal(rerr)
		}
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Kind != apperr.KindInvalidArgument || !strings.Contains(ae.Details["reason"], "cannot sit under") {
			t.Fatalf("moving the group under its own branch: %v, want a refusal that says why", err)
		}

		var op string
		must(t, "operator", q.QueryRow(ctx, `INSERT INTO platform_users (username, password_hash)
			VALUES ($1, 'x') RETURNING id::text`, "op-"+sfx).Scan(&op))
		_, err = q.Exec(ctx, `INSERT INTO platform_assignments (operator_id, tenant_id, role)
			VALUES ($1, $2, 'admin')`, op, child.ID)
		must(t, "assignment", err)
		mine, err := ctl.AssignmentsForOperator(ctx, op)
		if err != nil || len(mine) != 1 {
			t.Fatalf("assignments = %+v, %v", mine, err)
		}
		now := time.Now()
		if !mine[0].Covers(child.ID, now) || !mine[0].Covers(grand.ID, now) {
			t.Fatal("an admin of the subsidiary does not run it and its branch")
		}
		if mine[0].Covers(group.ID, now) {
			t.Fatal("an admin of the subsidiary runs the group above it")
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}
