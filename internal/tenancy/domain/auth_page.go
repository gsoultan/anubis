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
	// A page is bound to an application OR a realm, never both — resolution
	// would have to pick one and whichever it picked would surprise somebody.
	// The database refuses the row (auth_pages_one_binding, migration 0041).
	ApplicationID   string
	ApplicationSlug string
	// RealmID binds the page to a population: the door internal, partner or
	// public users see. Tried after the application binding, before the
	// tenant default.
	RealmID   string
	RealmCode string
	Config          []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
