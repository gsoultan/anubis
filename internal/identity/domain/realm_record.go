package identitydomain

// RealmRecord is the admin-plane realm shape. JSON-ish fields stay []byte
// (jsonb pass-through); the transport renders them, nothing in between
// parses them.
type RealmRecord struct {
	ID                string
	Code              string
	Kind              string
	DisplayName       string
	MinAssurance      int
	SelfRegistration  bool
	EmailVerification bool
	PIIEncryption     bool
	AllowedFactors    []string
	RequiredFactors   []string
	SessionTTL        string
	AccessTokenTTL    string
	RefreshTokenTTL   string
	DefaultRetention  string
	PasswordPolicy    []byte
}
