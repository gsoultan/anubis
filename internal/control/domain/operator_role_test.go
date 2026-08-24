package controldomain

import (
	"strings"
	"testing"
	"time"
)

// The allow-lists are the control plane's whole security surface. These
// assertions are here to make a change to them deliberate: if you are editing
// this test, you are widening what an operator may do.
func TestOperatorRolePermissionBoundaries(t *testing.T) {
	// Support administers a tenant's people. It does not touch what the
	// tenant shows the world, and it hands out no access.
	for _, p := range []string{"anubis:identity:read", "anubis:identity:write", "anubis:credential:write"} {
		if !RoleSupport.Allows(p) {
			t.Errorf("support should allow %s", p)
		}
	}
	for _, p := range []string{
		"anubis:signin:admin", "anubis:grant:admin", "anubis:role:admin",
		"anubis:audit:read", "anubis:apikey:admin", PermManageTenants, PermAssignOperators,
	} {
		if RoleSupport.Allows(p) {
			t.Errorf("support must NOT allow %s", p)
		}
	}

	// Admin is the operator proper: the tenant's people AND its sign-in
	// pages, which is the job the concept describes.
	for _, p := range []string{"anubis:identity:write", "anubis:signin:admin", "anubis:apikey:admin"} {
		if !RoleAdmin.Allows(p) {
			t.Errorf("admin should allow %s", p)
		}
	}

	// Owner runs the installation.
	for _, p := range []string{PermManageTenants, PermAssignOperators, "anubis:tenant:admin"} {
		if !RoleOwner.Allows(p) {
			t.Errorf("owner should allow %s", p)
		}
	}
}

// Creating and deleting tenants is the installation's business. An operator
// assigned to one tenant must not reach the others through it.
func TestOnlyOwnerCanManageTenantsOrOperators(t *testing.T) {
	for _, r := range []OperatorRole{RoleSupport, RoleAdmin} {
		for _, p := range []string{PermManageTenants, PermAssignOperators, "anubis:tenant:admin"} {
			if r.Allows(p) {
				t.Errorf("%s carries %s", r, p)
			}
		}
	}
}

// A role added to the schema but not to this file must inherit nothing.
func TestUnknownRoleFailsClosed(t *testing.T) {
	var r OperatorRole = "root"
	if r.Valid() {
		t.Error("root should not be a valid role")
	}
	if len(r.Permissions()) != 0 {
		t.Errorf("unknown role carries %v", r.Permissions())
	}
	for _, p := range RoleOwner.Permissions() {
		if r.Allows(p) {
			t.Errorf("unknown role allowed %s", p)
		}
	}
}

// Permissions hands out a copy; a caller mutating it must not be able to
// widen every future authorisation decision in the process.
func TestPermissionsCannotBeMutatedByCallers(t *testing.T) {
	got := RoleSupport.Permissions()
	got[0] = "anubis:tenant:admin"
	if RoleSupport.Allows("anubis:tenant:admin") {
		t.Fatal("mutating the returned slice changed what the role allows")
	}
}

func TestRoleValidity(t *testing.T) {
	for _, r := range []OperatorRole{RoleSupport, RoleAdmin, RoleOwner} {
		if !r.Valid() {
			t.Errorf("%s should be valid", r)
		}
	}
	for _, r := range []OperatorRole{"", "Admin", "superuser"} {
		if r.Valid() {
			t.Errorf("%q should not be valid", r)
		}
	}
}

func TestAssignmentCoverage(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)

	global := AssignmentRecord{Role: RoleOwner}
	if !global.Global() || !global.Covers("any-tenant", now) {
		t.Error("an empty tenant id means every tenant")
	}

	scoped := AssignmentRecord{TenantID: "t1", Role: RoleAdmin}
	if !scoped.Covers("t1", now) {
		t.Error("should cover its own tenant")
	}
	if scoped.Covers("t2", now) {
		t.Error("must not cover another tenant")
	}

	expired := AssignmentRecord{TenantID: "t1", ValidUntil: &past}
	if expired.Covers("t1", now) {
		t.Error("an expired assignment covers nothing")
	}
	if live := (AssignmentRecord{TenantID: "t1", ValidUntil: &future}); !live.Covers("t1", now) {
		t.Error("an unexpired assignment should cover its tenant")
	}
	revoked := AssignmentRecord{TenantID: "t1", RevokedAt: &past}
	if revoked.Covers("t1", now) {
		t.Error("a revoked assignment covers nothing")
	}
}

func validSetup() SetupInput {
	return SetupInput{
		Token: "t", DBHost: "localhost", DBPort: 5432, DBName: "anubis", DBUser: "anubis",
		DBSSLMode:     "require",
		OwnerUsername: "owner", OwnerPassword: strings.Repeat("x", MinOwnerPassword),
	}
}

func TestSetupInputValidation(t *testing.T) {
	if got := validSetup().Problems(); len(got) != 0 {
		t.Fatalf("valid input reported %v", got)
	}

	cases := map[string]func(*SetupInput){
		"token":          func(in *SetupInput) { in.Token = "  " },
		"db_host":        func(in *SetupInput) { in.DBHost = "" },
		"db_port":        func(in *SetupInput) { in.DBPort = 0 },
		"db_name":        func(in *SetupInput) { in.DBName = "" },
		"db_user":        func(in *SetupInput) { in.DBUser = "" },
		"db_sslmode":     func(in *SetupInput) { in.DBSSLMode = "sortof" },
		"owner_username": func(in *SetupInput) { in.OwnerUsername = "" },
		"owner_password": func(in *SetupInput) { in.OwnerPassword = strings.Repeat("x", MinOwnerPassword-1) },
	}
	for field, break_ := range cases {
		in := validSetup()
		break_(&in)
		if _, bad := in.Problems()[field]; !bad {
			t.Errorf("%s should have been reported: %v", field, in.Problems())
		}
	}

	// Out-of-range ports are a typo, not a port.
	for _, p := range []int{-1, 0, 65536} {
		in := validSetup()
		in.DBPort = p
		if _, bad := in.Problems()["db_port"]; !bad {
			t.Errorf("port %d should be rejected", p)
		}
	}

	// A half-filled first-tenant pair is a typo, not a choice.
	half := validSetup()
	half.FirstTenantName = "Acme"
	if _, bad := half.Problems()["first_tenant_slug"]; !bad {
		t.Error("a name without a slug should be reported")
	}

	// The first tenant is optional, and an installation with none is valid.
	none := validSetup()
	if got := none.Problems(); len(got) != 0 {
		t.Errorf("no first tenant should be fine, got %v", got)
	}

	// Testing the connection must not demand fields the form has not asked
	// for yet.
	dbOnly := SetupInput{Token: "t", DBHost: "h", DBPort: 5432, DBName: "n", DBUser: "u"}
	if got := dbOnly.DatabaseProblems(); len(got) != 0 {
		t.Errorf("database-only validation reported %v", got)
	}
	if _, bad := dbOnly.Problems()["owner_password"]; !bad {
		t.Error("full validation should still require an owner password")
	}

	// Every field is reported at once, so the form marks them all in one pass.
	empty := SetupInput{}
	if got := empty.Problems(); len(got) < 5 {
		t.Errorf("an empty input reported only %d problems: %v", len(got), got)
	}
}
