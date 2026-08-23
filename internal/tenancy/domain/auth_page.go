package tenancydomain

import "time"

// AuthPage is one sign-in or sign-out page. A tenant may have many of each,
// addressed by slug; exactly one per kind is the default that /v1/authorize
// falls back to.
type AuthPage struct {
	ID              string
	TenantID        string
	Kind            string // signin | signout
	Slug            string
	Name            string
	Status          string // active | disabled
	IsDefault       bool
	ApplicationID   string
	ApplicationSlug string
	Config          []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
