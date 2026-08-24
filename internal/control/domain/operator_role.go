// Package controldomain is the platform control plane (ADR-0011): who may
// operate this installation, and over which tenants.
//
// It is deliberately separate from the data plane. Nothing here decides what
// a tenant's own members may do — that stays with grants and authorize().
package controldomain

// OperatorRole is what a platform operator may do inside a tenant their
// assignment covers.
type OperatorRole string

const (
	// RoleSupport administers a tenant's people, and nothing else.
	RoleSupport OperatorRole = "support"
	// RoleAdmin administers a tenant: its people and the sign-in pages they
	// are shown, plus the rest of that tenant's configuration.
	RoleAdmin OperatorRole = "admin"
	// RoleOwner runs the installation: the tenants that exist, and who
	// operates it. Not a bigger operator — a different job.
	RoleOwner OperatorRole = "owner"
)

// Control-plane permissions. They are not anubis:* tenant permissions
// because they are not things a tenant's own members can ever be granted.
const (
	// PermAssignOperators creates operators and assigns them to tenants.
	PermAssignOperators = "anubis:platform:assign"
	// PermManageTenants creates, modifies and deletes tenants — the
	// installation itself, which no operator scoped to one tenant may touch.
	PermManageTenants = "anubis:platform:tenants"
)

// operatorPermissions is what an OPERATOR carries inside a tenant they are
// assigned to: that tenant's people, and the sign-in pages those people see.
//
// Deliberately an allow-list, not a prefix rule. "Operators get anubis:*"
// reads as generous today and then silently absorbs every permission added
// afterwards — including ones nobody meant an operator to have.
var operatorPermissions = []string{
	// The tenant's people.
	"anubis:identity:read",
	"anubis:identity:write",
	"anubis:credential:write",
	// The sign-in pages those people are shown.
	"anubis:signin:admin",
}

// supportPermissions is the narrower half: read the tenant's people and
// administer them, but not the pages the tenant presents to the world.
var supportPermissions = []string{
	"anubis:identity:read",
	"anubis:identity:write",
	"anubis:credential:write",
}

// adminPermissions is everything an operator may do inside one tenant.
//
// anubis:tenant:admin is absent on purpose: creating and deleting tenants is
// the installation's business, and an operator assigned to a single tenant
// must not reach the others through it. That belongs to the owner alone.
var adminPermissions = append([]string{
	"anubis:audit:read",
	"anubis:consent:write",
	"anubis:grant:admin",
	"anubis:membership:admin",
	"anubis:realm:admin",
	"anubis:role:admin",
	"anubis:scope:admin",
	"anubis:sync:admin",
	"anubis:application:admin",
	// The tenant's machine credentials (0030). Beside application:admin on
	// purpose: keys and relying parties are the same job — the tenant's
	// integration surface — and support gets neither.
	"anubis:apikey:admin",
	"anubis:manifest:apply",
}, operatorPermissions...)

// ownerPermissions is the installation itself: the tenants that exist, and
// who operates it. An owner is not a bigger operator — they answer a
// different question.
var ownerPermissions = append([]string{
	PermAssignOperators,
	PermManageTenants,
	"anubis:tenant:admin",
	"anubis:key:admin",
}, adminPermissions...)

// Valid reports whether r is a role the schema will accept.
func (r OperatorRole) Valid() bool {
	switch r {
	case RoleSupport, RoleAdmin, RoleOwner:
		return true
	}
	return false
}

// Allows reports whether this role carries a permission inside an assigned
// tenant. An unknown role allows nothing — a role added to the schema but
// not to this file fails closed rather than inheriting someone else's rights.
func (r OperatorRole) Allows(permission string) bool {
	for _, p := range r.permissions() {
		if p == permission {
			return true
		}
	}
	return false
}

func (r OperatorRole) permissions() []string {
	switch r {
	case RoleSupport:
		return supportPermissions
	case RoleAdmin:
		return adminPermissions
	case RoleOwner:
		return ownerPermissions
	default:
		return nil
	}
}

// Permissions is the role's allow-list, for display and for tests that pin
// what each role carries.
func (r OperatorRole) Permissions() []string {
	src := r.permissions()
	out := make([]string, len(src))
	copy(out, src)
	return out
}
