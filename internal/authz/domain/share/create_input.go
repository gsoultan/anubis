package share

import "time"

// CreateInput is what the repository stores. Validation happened before.
type CreateInput struct {
	TenantID       string
	FromIdentityID string
	ToIdentityID   string
	Record         string
	RecordScopes   map[string]string
	Permissions    []string
	Note           string
	ValidUntil     time.Time
}
