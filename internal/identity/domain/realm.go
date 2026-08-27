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
	// FactorEnrolmentDeadline is when RequiredFactors starts being enforced
	// against members who have not enrolled. Zero means never — which is
	// every realm until somebody sets a date. See EnrolmentStance.
	FactorEnrolmentDeadline time.Time
}

// EnrolmentStance is what a realm does about a member who has not enrolled a
// factor the realm requires.
type EnrolmentStance int

const (
	// EnrolmentNotEnforced: the realm requires nothing this member lacks, or
	// no deadline has been set. Sign in normally.
	EnrolmentNotEnforced EnrolmentStance = iota
	// EnrolmentDue: the policy is in force but the deadline has not passed.
	// Sign in works and the response carries the date, so the member is told
	// before it costs them access rather than after.
	EnrolmentDue
	// EnrolmentOverdue: enrol-or-deny. No session is issued, and the refusal
	// carries a way to enrol — a policy that locks people out without telling
	// them how to comply is a support incident with extra steps.
	EnrolmentOverdue
)

// EnrolmentStanceFor decides what to do about a member holding `enrolled`
// factors, at time `now`.
//
// The stance is about factors the realm REQUIRES and the member LACKS.
// Anything they have enrolled is demanded anyway, by a separate rule, and a
// factor the realm no longer allows is not required by it either.
func (r *Realm) EnrolmentStanceFor(enrolled []string, now time.Time) (EnrolmentStance, []string) {
	if r.FactorEnrolmentDeadline.IsZero() {
		return EnrolmentNotEnforced, nil
	}
	var missing []string
	for _, want := range r.RequiredFactors {
		// A password is not a second factor and is verified before this
		// point; requiring it here would demand enrolment of everybody.
		if want == "password" || !r.AllowsFactor(want) {
			continue
		}
		if !contains(enrolled, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return EnrolmentNotEnforced, nil
	}
	if now.Before(r.FactorEnrolmentDeadline) {
		return EnrolmentDue, missing
	}
	return EnrolmentOverdue, missing
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
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
