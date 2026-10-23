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
