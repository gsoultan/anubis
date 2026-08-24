package tenancyapp

import (
	"testing"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
)

// The tenancy context cannot import the control context's constant without
// pointing the dependency the wrong way, so it repeats the string. If the two
// drift, tenant creation silently becomes unreachable for the only people who
// are supposed to do it.
func TestManageTenantsPermissionMatchesTheControlPlane(t *testing.T) {
	if permManageTenants != controldomain.PermManageTenants {
		t.Fatalf("tenancy checks %q, control grants %q", permManageTenants, controldomain.PermManageTenants)
	}
	// And it must be owner-only.
	if controldomain.RoleAdmin.Allows(permManageTenants) || controldomain.RoleSupport.Allows(permManageTenants) {
		t.Fatal("an operator must not be able to manage tenants")
	}
	if !controldomain.RoleOwner.Allows(permManageTenants) {
		t.Fatal("the owner must be able to manage tenants")
	}
}

// Same drift risk as the tenant permission: the tenancy interactor checks a
// string the control plane grants. If they part ways, API keys silently
// become uncreatable for the only people allowed to create them.
func TestAPIKeyPermissionIsAdminNotSupport(t *testing.T) {
	if !controldomain.RoleAdmin.Allows(permAPIKeys) {
		t.Fatal("an operator admin must be able to manage the tenant's API keys")
	}
	if controldomain.RoleSupport.Allows(permAPIKeys) {
		t.Fatal("support must not mint machine credentials")
	}
}
