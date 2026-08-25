package identitydomain

import "time"

// Realm is population policy: how its members sign in and how long they stay.
type Realm struct {
	ID                string
	TenantID          string
	Code              string
	Kind              string // internal | partner | public | service
	DisplayName       string
	MinAssurance      int
	SelfRegistration  bool
	EmailVerification bool
	AllowedFactors    []string
	RequiredFactors   []string
	SessionTTL        time.Duration
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	PasswordPolicy    PasswordPolicy
}

// RequiresFactor reports whether realm policy demands a factor beyond the
// primary password (e.g. "totp").
func (r *Realm) RequiresFactor(factor string) bool {
	for _, f := range r.RequiredFactors {
		if f == factor {
			return true
		}
	}
	return false
}

func (r *Realm) AllowsFactor(factor string) bool {
	for _, f := range r.AllowedFactors {
		if f == factor {
			return true
		}
	}
	return false
}

// RealmCount is one population's size — the dashboard's by-realm figure.
type RealmCount struct {
	Realm string
	Kind  string
	Count int64
}
