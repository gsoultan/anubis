//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	rrole "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen/role"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// The full-context acceptance test for the M6 migration: every repository
// family runs through the storm-backed methods against real data. PREPARE at
// generate time proves each statement's SQL and row shape; what it CANNOT
// prove is argument order at the call sites, so each family here asserts
// values, and every write happens inside a deliberately rolled-back ambient
// transaction.
var errRollback = errors.New("deliberate rollback")

func TestStormFull_RoleFamily(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()

	roles, err := repo.ListRoles(ctx, tenant, "")
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM roles WHERE tenant_id = $1`, tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if int64(len(roles)) != n {
		t.Fatalf("ListRoles: %d rows, database has %d", len(roles), n)
	}
	if len(roles) == 0 {
		t.Skip("tenant has no roles")
	}

	// GetRole's (id, tenant) argument order, checked by value.
	rec, err := repo.RoleByID(ctx, tenant, roles[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != roles[0].ID || rec.Name != roles[0].Name {
		t.Fatalf("RoleByID(%s) returned role %s", roles[0].ID, rec.ID)
	}
	if _, err := repo.RoleByID(ctx, tenant, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing role: %v, want ErrNotFound", err)
	}

	err = repo.WithinTx(ctx, func(ctx context.Context) error {
		id, err := repo.CreateRole(ctx, tenant, authzdomain.RoleRecord{
			Name: "storm-full-role", Description: "before",
		}, "")
		if err != nil {
			return err
		}
		// UpdateRole (id, tenant, desc, kinds, assignable) order.
		if err := repo.UpdateRole(ctx, tenant, authzdomain.RoleRecord{
			ID: id, Description: "after", AllowedRealmKinds: []string{"internal"},
		}); err != nil {
			return err
		}
		got, err := repo.RoleByID(ctx, tenant, id)
		if err != nil {
			return err
		}
		if got.Description != "after" {
			t.Fatalf("UpdateRole did not land: description %q", got.Description)
		}

		// Composition: parents, descendants, patterns, permissions.
		parent := roles[0].ID
		if err := repo.SetRoleParents(ctx, id, []string{parent}); err != nil {
			return err
		}
		ps, err := repo.RoleParents(ctx, id)
		if err != nil {
			return err
		}
		if len(ps) != 1 || ps[0] != parent {
			t.Fatalf("RoleParents = %v, want [%s]", ps, parent)
		}
		below, err := repo.RolesBelow(ctx, parent)
		if err != nil {
			return err
		}
		found := false
		for _, b := range below {
			if b == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("RolesBelow(%s) = %v does not contain the new child %s", parent, below, id)
		}

		if err := repo.SetRolePatterns(ctx, id, []string{"app:*:read"}); err != nil {
			return err
		}
		pats, err := repo.RolePatterns(ctx, id)
		if err != nil {
			return err
		}
		if len(pats) != 1 || pats[0] != "app:*:read" {
			t.Fatalf("RolePatterns = %v", pats)
		}
		using, err := repo.RolesUsingPatterns(ctx, tenant)
		if err != nil {
			return err
		}
		found = false
		for _, u := range using {
			if u == id {
				found = true
			}
		}
		if !found {
			t.Fatal("RolesUsingPatterns does not list the role just given a pattern")
		}

		if err := repo.RecomputeRole(ctx, id); err != nil {
			return err
		}
		if _, err := repo.RoleEffective(ctx, id); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}

func TestStormFull_GrantFamily(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()

	var identity, roleID string
	if err := pool.QueryRow(ctx, `
		SELECT g.identity_id::text, g.role_id::text FROM grants g
		 WHERE g.tenant_id = $1 LIMIT 1`, tenant).Scan(&identity, &roleID); err != nil {
		t.Skipf("no grant data: %v", err)
	}

	// Read side against live data.
	grants, err := repo.ListGrants(ctx, tenant, identity, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) == 0 {
		t.Fatal("ListGrants returned nothing for an identity that has a grant")
	}
	ids := make([]string, 0, len(grants))
	for _, g := range grants {
		if g.ID == "" || g.RoleName == "" {
			t.Fatalf("grant with empty id or role name: %+v", g)
		}
		ids = append(ids, g.ID)
	}
	if _, err := repo.GrantScopes(ctx, ids); err != nil {
		t.Fatal(err)
	}

	hits, err := repo.SearchGrants(ctx, tenant, grant.GrantSearch{IdentityID: identity, IncludeRevoked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != len(grants) {
		t.Fatalf("SearchGrants by identity found %d, ListGrants found %d", len(hits), len(grants))
	}
	if _, err := repo.CountLiveGrants(ctx, tenant); err != nil {
		t.Fatal(err)
	}

	// Write side, rolled back.
	err = repo.WithinTx(ctx, func(ctx context.Context) error {
		id, err := repo.CreateGrant(ctx, grant.GrantCreate{
			TenantID: tenant, IdentityID: identity, RoleID: roleID,
			GrantedBy: identity, Reason: "storm full test",
		})
		if err != nil {
			return err
		}
		after, err := repo.ListGrants(ctx, tenant, identity, false)
		if err != nil {
			return err
		}
		var mine *grant.GrantRecord
		for i := range after {
			if after[i].ID == id {
				mine = &after[i]
			}
		}
		if mine == nil {
			t.Fatalf("created grant %s not listed inside the tx", id)
		}
		if mine.Reason != "storm full test" || mine.RoleID != roleID {
			t.Fatalf("grant round-trip lost values: %+v", mine)
		}
		if err := repo.RevokeGrant(ctx, tenant, id, "undone"); err != nil {
			return err
		}
		live, err := repo.ListGrants(ctx, tenant, identity, false)
		if err != nil {
			return err
		}
		for _, g := range live {
			if g.ID == id {
				t.Fatal("revoked grant still listed as live")
			}
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}

func TestStormFull_MembershipFamily(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()

	var identity, roleID string
	if err := pool.QueryRow(ctx, `
		SELECT g.identity_id::text, g.role_id::text FROM grants g
		 WHERE g.tenant_id = $1 LIMIT 1`, tenant).Scan(&identity, &roleID); err != nil {
		t.Skipf("no seed data: %v", err)
	}

	err := repo.WithinTx(ctx, func(ctx context.Context) error {
		mid, err := repo.CreateMembership(ctx, tenant, "storm-full-membership", "probe")
		if err != nil {
			return err
		}
		m, err := repo.MembershipByID(ctx, tenant, mid)
		if err != nil {
			return err
		}
		if m.Name != "storm-full-membership" {
			t.Fatalf("MembershipByID name %q", m.Name)
		}
		all, err := repo.ListMemberships(ctx, tenant)
		if err != nil {
			return err
		}
		found := false
		for _, mm := range all {
			if mm.ID == mid {
				found = true
			}
		}
		if !found {
			t.Fatal("ListMemberships misses the new membership")
		}

		if err := repo.ReplaceMembershipEntries(ctx, tenant, mid, []membership.MembershipEntryInput{
			{RoleID: roleID},
		}); err != nil {
			return err
		}
		entries, err := repo.MembershipEntries(ctx, []string{mid})
		if err != nil {
			return err
		}
		if len(entries) != 1 || entries[0].RoleID != roleID {
			t.Fatalf("MembershipEntries = %+v", entries)
		}
		if _, err := repo.MembershipEntryScopes(ctx, []string{entries[0].ID}); err != nil {
			return err
		}

		created, err := repo.AssignMembership(ctx, identity, mid, identity)
		if err != nil {
			return err
		}
		if created < 1 {
			t.Fatalf("AssignMembership materialized %d grants, want at least 1", created)
		}
		changed, err := repo.ResyncMembership(ctx, mid)
		if err != nil {
			return err
		}
		_ = changed // resync of a fresh assignment may touch nothing
		revoked, err := repo.UnassignMembership(ctx, identity, mid)
		if err != nil {
			return err
		}
		if revoked != created {
			t.Fatalf("unassign revoked %d of the %d grants assign created", revoked, created)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}

func TestStormFull_PermissionFamily(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()

	perms, err := repo.ListPermissions(ctx, tenant, "", true)
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE tenant_id = $1`, tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if int64(len(perms)) != n {
		t.Fatalf("ListPermissions: %d rows, database has %d", len(perms), n)
	}
	if len(perms) == 0 {
		t.Skip("tenant has no permissions")
	}
	live := ""
	for _, p := range perms {
		if !p.Deprecated && p.Key != "" {
			live = p.Key
			break
		}
	}
	if live != "" {
		id, err := repo.PermissionIDByKey(ctx, tenant, live)
		if err != nil {
			t.Fatal(err)
		}
		meta, err := repo.PermissionByKey(ctx, tenant, live)
		if err != nil {
			t.Fatal(err)
		}
		if meta.ID != id || meta.Key != live {
			t.Fatalf("PermissionByKey/PermissionIDByKey disagree: %s vs %s", meta.ID, id)
		}
	}

	var appID, appSlug string
	if err := pool.QueryRow(ctx, `
		SELECT p.application_id::text, p.app_slug FROM permissions p
		 WHERE p.tenant_id = $1 LIMIT 1`, tenant).Scan(&appID, &appSlug); err != nil {
		t.Skipf("no application to upsert under: %v", err)
	}
	err = repo.WithinTx(ctx, func(ctx context.Context) error {
		id, key, err := repo.UpsertPermission(ctx, tenant, appID, appSlug, authzdomain.PermissionRecord{
			Resource: "stormfull", Action: "probe", Description: "acceptance probe",
		})
		if err != nil {
			return err
		}
		if id == "" || key == "" {
			t.Fatalf("UpsertPermission returned id=%q key=%q", id, key)
		}
		keep := []string{}
		for _, p := range perms {
			if p.AppSlug == appSlug && !p.Deprecated {
				keep = append(keep, p.ID)
			}
		}
		// Deprecate everything except the pre-existing set: exactly the probe.
		gone, err := repo.DeprecatePermissionsExcept(ctx, appID, keep)
		if err != nil {
			return err
		}
		if len(gone) != 1 || gone[0] != key {
			t.Fatalf("DeprecatePermissionsExcept deprecated %v, want [%s]", gone, key)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("WithinTx: %v", err)
	}
}

func TestStormFull_ExplainAndSim(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()
	identity, permission, targets := realDecisionProbe(ctx, t, tenant)
	raw := mustJSON(t, targets)

	detail, err := repo.AuthorizeExplain(ctx, identity, tenant, permission, raw)
	if err != nil {
		t.Fatal(err)
	}
	if detail == "" {
		t.Fatal("AuthorizeExplain returned an empty narration")
	}

	// Axis-unchanged strict sim must agree with the engine (the parity the
	// SQL's own comment promises).
	allow, err := repo.Authorize(ctx, identity, tenant, permission, raw)
	if err != nil {
		t.Fatal(err)
	}
	sim, err := repo.AuthorizeStrictSim(ctx, identity, tenant, permission, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if sim != allow {
		t.Fatalf("strict sim with no flipped axis says %v, engine says %v", sim, allow)
	}

	if _, err := repo.EffectivePermissionsForIdentity(ctx, tenant, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SampleAuthorizeDecisions(ctx, tenant, 5); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The thesis, asserted at the adopter rather than in storm's own fixtures:
// a query's identity is its STRUCTURE, so a thousand lookups with a thousand
// different names must compile one statement, not a thousand. This is also
// the soak signal for P1.1 — if a repository method ever mints shapes from
// request data, the count grows here first and the shape cache starts
// flushing in production later.
func TestStormFull_VaryingValuesDoNotMintShapes(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	repo := sliceRepo()

	// Warm whatever this call site compiles, then measure from there.
	if _, err := repo.RoleByName(ctx, tenant, "warm-up"); err != nil && !errors.Is(err, apperr.ErrNotFound) {
		t.Fatal(err)
	}
	before, flushesBefore := rrole.Shapes(), rrole.ShapeFlushes()

	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("storm-shape-probe-%d", i)
		if _, err := repo.RoleByName(ctx, tenant, name); err != nil && !errors.Is(err, apperr.ErrNotFound) {
			t.Fatal(err)
		}
	}

	after, flushesAfter := rrole.Shapes(), rrole.ShapeFlushes()
	t.Logf("500 lookups with 500 distinct names: shapes %d → %d, flushes %d → %d",
		before, after, flushesBefore, flushesAfter)

	if after != before {
		t.Fatalf("500 distinct VALUES compiled %d new statements — a value is leaking into the query structure",
			after-before)
	}
	if flushesAfter != flushesBefore {
		t.Fatalf("the shape cache flushed during a fixed-structure workload (%d → %d)",
			flushesBefore, flushesAfter)
	}
}
