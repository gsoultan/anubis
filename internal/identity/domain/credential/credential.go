package credential

type Credential struct {
	ID         string
	IdentityID string
	TenantID   string
	Kind       string
	Secret     string
	// SecretKid names the local key Secret was sealed under. Empty means the
	// enrolment predates the column and the reader must fall back.
	SecretKid   string
	Params      []byte
	SignCounter int64
}
