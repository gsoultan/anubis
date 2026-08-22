package identitydomain

import "github.com/gsoultan/anubis/internal/shared/apperr"

// Identity is the authenticated subject.
type Identity struct {
	ID             string
	TenantID       string
	RealmID        string
	RealmCode      string
	RealmKind      string
	Username       string
	Email          string
	Status         string
	AssuranceLevel int
	TokenEpoch     int
	Disabled       bool
	Anonymized     bool
}

// CanAuthenticate is the identity-state gate at the front door — the same
// rule authorize() applies inside the database (0009 gate 1).
func (i *Identity) CanAuthenticate() error {
	switch {
	case i.Anonymized, i.Disabled, i.Status == "disabled":
		return apperr.ErrIdentityDisabled
	case i.Status == "locked":
		return apperr.ErrIdentityLocked
	case i.Status != "active":
		return apperr.ErrInvalidCredentials
	}
	return nil
}
