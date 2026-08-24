package guard

import (
	"context"
	"testing"
	"time"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

type fakeOps struct {
	rows []controldomain.AssignmentRecord
}

func (f fakeOps) AssignmentsForOperator(context.Context, string) ([]controldomain.AssignmentRecord, error) {
	return f.rows, nil
}

func ctxFor(p *authctx.Principal) context.Context {
	return authctx.With(context.Background(), p)
}

// An owner creating the FIRST tenant has none selected — there are none to
// select. Requiring one made a fresh installation impossible to populate.
func TestOwnerActsOnTheInstallationWithNoTenantSelected(t *testing.T) {
	g := (&Guard{}).WithOperators(fakeOps{rows: []controldomain.AssignmentRecord{
		{OperatorID: "op", Role: controldomain.RoleOwner}, // global: no TenantID
	}}, time.Now)

	p := &authctx.Principal{IdentityID: "op", Platform: true}
	if _, err := g.Require(ctxFor(p), controldomain.PermManageTenants); err != nil {
		t.Fatalf("owner refused with no tenant selected: %v", err)
	}
}

// An operator's authority IS scoped to a tenant, so without one selected
// there is nothing to authorise them against.
func TestScopedOperatorNeedsATenantSelected(t *testing.T) {
	g := (&Guard{}).WithOperators(fakeOps{rows: []controldomain.AssignmentRecord{
		{OperatorID: "op", TenantID: "t1", Role: controldomain.RoleAdmin},
	}}, time.Now)

	p := &authctx.Principal{IdentityID: "op", Platform: true}
	if _, err := g.Require(ctxFor(p), "anubis:identity:read"); err == nil {
		t.Fatal("a scoped operator should not act without a tenant")
	}
	withTenant := &authctx.Principal{IdentityID: "op", TenantID: "t1", Platform: true}
	if _, err := g.Require(ctxFor(withTenant), "anubis:identity:read"); err != nil {
		t.Fatalf("refused inside their own tenant: %v", err)
	}
	elsewhere := &authctx.Principal{IdentityID: "op", TenantID: "t2", Platform: true}
	if _, err := g.Require(ctxFor(elsewhere), "anubis:identity:read"); err == nil {
		t.Fatal("an operator reached a tenant they are not assigned to")
	}
}

// And a scoped operator never gains installation-level authority, selected
// tenant or not.
func TestScopedOperatorCannotManageTheInstallation(t *testing.T) {
	g := (&Guard{}).WithOperators(fakeOps{rows: []controldomain.AssignmentRecord{
		{OperatorID: "op", TenantID: "t1", Role: controldomain.RoleAdmin},
	}}, time.Now)
	for _, p := range []*authctx.Principal{
		{IdentityID: "op", Platform: true},
		{IdentityID: "op", TenantID: "t1", Platform: true},
	} {
		if _, err := g.Require(ctxFor(p), controldomain.PermManageTenants); err == nil {
			t.Fatal("an operator managed the installation")
		}
	}
}

// An expired or revoked assignment is not authority.
func TestDeadAssignmentsGrantNothing(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	g := (&Guard{}).WithOperators(fakeOps{rows: []controldomain.AssignmentRecord{
		{OperatorID: "op", Role: controldomain.RoleOwner, ValidUntil: &past},
		{OperatorID: "op", Role: controldomain.RoleOwner, RevokedAt: &past},
	}}, time.Now)
	p := &authctx.Principal{IdentityID: "op", Platform: true}
	if _, err := g.Require(ctxFor(p), controldomain.PermManageTenants); err == nil {
		t.Fatal("a dead assignment authorised an action")
	}
}

// A tenant identity is a different population, not a lesser operator. However
// many roles it holds, the admin plane refuses it before any policy lookup —
// the rows that could have conferred administration no longer exist (0029),
// and this pins the refusal so they cannot quietly come back.
func TestTenantIdentityIsRefusedOutright(t *testing.T) {
	g := (&Guard{}).WithOperators(fakeOps{rows: []controldomain.AssignmentRecord{
		{OperatorID: "someone-else", Role: controldomain.RoleOwner},
	}}, time.Now)
	p := &authctx.Principal{IdentityID: "alice", TenantID: "t1", Roles: []string{"anything"}}
	if _, err := g.Require(ctxFor(p), "anubis:identity:read"); err == nil {
		t.Fatal("a tenant identity passed the admin guard")
	}
}
