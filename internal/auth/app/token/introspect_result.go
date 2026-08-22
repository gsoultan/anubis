package tokenapp

// IntrospectResult mirrors the api.md introspection response. Inactive
// tokens carry Active=false and nothing else — a dead token reveals nothing.
type IntrospectResult struct {
	Active   bool
	Subject  string
	Session  string
	Tenant   string
	Realm    string
	Roles    []string
	Scopes   map[string]string
	AMR      []string
	Audience []string
	Expires  int64
	AuthTime int64
	IAL      int
	Epoch    int
}
