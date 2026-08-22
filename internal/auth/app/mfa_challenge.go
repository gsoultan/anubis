package authapp

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
