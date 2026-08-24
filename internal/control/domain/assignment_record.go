package controldomain

import "time"

// AssignmentRecord is one operator's authority over one tenant, or over the
// whole installation when TenantID is empty.
type AssignmentRecord struct {
	ID         string
	OperatorID string
	// TenantID empty means every tenant: the installation owner.
	TenantID string
	// TenantSlug is filled in for display; it is empty for a global
	// assignment, which is the same thing an empty TenantID says.
	TenantSlug string
	Role       OperatorRole
	GrantedBy  string
	Reason     string
	ValidUntil *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Global reports whether this assignment covers every tenant.
func (a AssignmentRecord) Global() bool { return a.TenantID == "" }

// Live reports whether the assignment is in force at the given moment.
// Revocation and expiry are both checked here so no caller has to remember
// that an assignment has two ways of ending.
func (a AssignmentRecord) Live(now time.Time) bool {
	if a.RevokedAt != nil {
		return false
	}
	if a.ValidUntil != nil && !a.ValidUntil.After(now) {
		return false
	}
	return true
}

// Covers reports whether this assignment gives authority over a tenant.
func (a AssignmentRecord) Covers(tenantID string, now time.Time) bool {
	if !a.Live(now) {
		return false
	}
	return a.Global() || a.TenantID == tenantID
}
