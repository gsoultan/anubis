package controldomain

import "time"

// PlatformUser is somebody who operates this installation.
//
// Deliberately unrelated to identitydomain.Identity. An operator has no
// tenant, no realm, no grants, no scope and no retention deadline — none of
// those concepts apply to the person running the system rather than using it.
// Nothing joins this to identities, so a tenant's user cannot be promoted
// into an operator and an operator cannot be given a role inside a tenant.
//
// One consequence worth stating: a platform username is globally unique,
// because there is no tenant to scope it by. That is why signing in to the
// console needs a username and a password and nothing else.
type PlatformUser struct {
	ID          string
	Username    string
	Email       string
	Status      string
	TokenEpoch  int
	CreatedAt   time.Time
	LastLoginAt *time.Time
	DisabledAt  *time.Time
	Assignments []AssignmentRecord

	// TOTPEnrolledAt is nil until a code has been verified. Holding a secret
	// is not the same as having enrolled — a half-finished enrolment must
	// never start demanding a factor the operator cannot produce.
	TOTPEnrolledAt *time.Time
	// TOTPLastStep is the newest step accepted, so a code cannot be replayed
	// inside its own validity window.
	TOTPLastStep uint64
}

// MFAEnrolled reports whether this operator has a second factor. Once they
// do it is always demanded, matching the rule already in force for tenant
// identities.
func (u PlatformUser) MFAEnrolled() bool { return u.TOTPEnrolledAt != nil }

// Active reports whether this operator may still sign in.
func (u PlatformUser) Active() bool { return u.Status == "active" }

// Owner reports whether this operator has authority over every tenant.
func (u PlatformUser) Owner(now time.Time) bool {
	for _, a := range u.Assignments {
		if a.Global() && a.Role == RoleOwner && a.Live(now) {
			return true
		}
	}
	return false
}

// Page is one page of a listing, with the cursor that fetches the next.
//
// Keyset, not offset: these tables grow, and OFFSET re-scans everything it
// skips and can show the same row twice when one is inserted mid-page.
type Page struct {
	Users []PlatformUser
	// NextCursor is empty when the last page has been reached.
	NextCursor string
	// Total is the whole population, so a screen can say "50 of 4,812"
	// instead of implying the page is everything there is.
	Total int
}
