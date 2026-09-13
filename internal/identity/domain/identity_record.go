package identitydomain

import "time"

type IdentityRecord struct {
	ID             string
	Username       string
	Email          string
	RealmCode      string
	RealmKind      string
	Status         string
	Category       string
	ExternalRef    string
	AssuranceLevel int
	TokenEpoch     int
	CreatedAt      time.Time
	LastLoginAt    *time.Time
	DisabledAt     *time.Time
	AnonymizedAt   *time.Time
	// RetentionUntil is when the retention sweeper may anonymise this
	// identity. nil = no statutory limit, which is nearly every employee.
	RetentionUntil *time.Time
}
