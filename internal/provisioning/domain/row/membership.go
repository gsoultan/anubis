package row

import "time"

// Membership is one person to be added to one membership.
type Membership struct {
	Row      int
	Realm    string
	Username string
	Name     string
	// PlaceRef is the external reference of the place, for a membership that
	// applies where each member is assigned; its axis is the membership's own.
	PlaceRef string
	// Exact limits the place to itself; the default covers everything inside.
	Exact      bool
	ValidUntil *time.Time
	Reason     string
}
