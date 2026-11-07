package tenancydomain

import "time"

type TenantRef struct {
	ID        string
	Slug      string
	Name      string
	Status    string
	CreatedAt time.Time
	// ParentID is the company that owns this one; "" at the top.
	ParentID string
}

// Serving reports whether the tenant's people may sign in and use what they
// were issued. A suspended or archived tenant keeps its rows and its history;
// it stops serving (ADR-0011: suspending a tenant must invalidate its tokens).
func (t *TenantRef) Serving() bool { return t != nil && t.Status == "active" }
