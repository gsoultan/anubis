package authapp

import "time"

// MFAChallenge is the 202 shape of a login that needs a second factor.
type MFAChallenge struct {
	MFAToken  string
	Methods   []string
	ExpiresIn int
}

// Authentication methods, RFC 8176 values, shared by every sign-in path.
const (
	AMRPassword = "pwd"
	AMROTP      = "otp"
	AMRDevice   = "device_key"
)

// MFAState is the encrypted payload inside an anb.local.v1 MFA token: the
// half-finished login it resumes.
type MFAState struct {
	TenantID   string   `json:"tenant_id"`
	TenantSlug string   `json:"tenant_slug"`
	IdentityID string   `json:"identity_id"`
	RealmID    string   `json:"realm_id"`
	ClientID   string   `json:"client_id"`
	DeviceFP   string   `json:"device_fp"`
	Methods    []string `json:"methods"`
	IP         string   `json:"ip"`
	UserAgent  string   `json:"ua"`
}

// EnrolmentChallenge is what a member is told about a factor their realm
// requires and they have not enrolled.
//
// It appears in two places and means two things. Alongside tokens it is a
// warning: sign-in worked, and here is the date. On its own it is a refusal
// that carries a way to comply — GrantToken stands in for the session the
// policy is withholding, because demanding a session to enrol would make the
// policy unsatisfiable by exactly the people it applies to.
type EnrolmentChallenge struct {
	Factors    []string
	Deadline   time.Time
	GrantToken string
	ExpiresIn  int
}

// EnrolmentGrant is the encrypted payload inside an enrolment grant token: a
// password that was accepted, and nothing else.
//
// It deliberately carries no session id and no scopes. Its whole authority is
// "enrol a factor for this identity", which is the only thing its holder is
// being asked to do.
type EnrolmentGrant struct {
	TenantID   string   `json:"tenant_id"`
	TenantSlug string   `json:"tenant_slug"`
	IdentityID string   `json:"identity_id"`
	RealmID    string   `json:"realm_id"`
	Factors    []string `json:"factors"`
}
