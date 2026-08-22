package repository

type Credential struct {
	ID          string
	IdentityID  string
	TenantID    string
	Kind        string
	Secret      string
	Params      []byte
	SignCounter int64
}
