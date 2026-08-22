package usecase

// Me is the /v1/me shape.
type Me struct {
	IdentityID   string
	Tenant       string
	Realm        string
	Username     string
	Email        string
	Roles        []string
	Permissions  []string
	ActiveScopes map[string]string
	AMR          []string
	IAL          int
	SessionID    string
}
