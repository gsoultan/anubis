package controldomain

import "time"

// OperatorRecord is one person who can operate this installation, together
// with the assignments that say where.
//
// An operator is an ordinary identity in the platform tenant — the owner that
// setup creates is simply the first one — so the identity half of this comes
// from the identity context unchanged.
type OperatorRecord struct {
	IdentityID  string
	Username    string
	Email       string
	Status      string
	CreatedAt   time.Time
	LastLoginAt *time.Time
	Assignments []AssignmentRecord
}

// Owner reports whether this operator has authority over every tenant.
func (o OperatorRecord) Owner(now time.Time) bool {
	for _, a := range o.Assignments {
		if a.Global() && a.Role == RoleOwner && a.Live(now) {
			return true
		}
	}
	return false
}
