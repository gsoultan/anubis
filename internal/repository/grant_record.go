package repository

import "time"

type GrantRecord struct {
	ID              string
	IdentityID      string
	RoleID          string
	RoleName        string
	SelfScoped      bool
	ValidFrom       time.Time
	ValidUntil      *time.Time
	RevokedAt       *time.Time
	GrantedBy       string
	ViaMembershipID string
	Reason          string
}
