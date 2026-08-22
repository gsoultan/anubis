package repository

import "time"

type GrantCreate struct {
	TenantID   string
	IdentityID string
	RoleID     string
	GrantedBy  string
	Reason     string
	SelfScoped bool
	ValidUntil *time.Time
	Scopes     []GrantScopeInput
}
