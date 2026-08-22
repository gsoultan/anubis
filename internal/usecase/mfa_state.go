package usecase

// mfaState is the encrypted payload inside an anb.local.v1 MFA token: the
// half-finished login it resumes.
type mfaState struct {
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
