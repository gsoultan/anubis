package usecase

// Manifest is the registration document applications ship
// (docs/api.md §Manifests). Applications own their catalogs; Anubis
// validates and stores, it does not curate.
type Manifest struct {
	Permissions []ManifestPermission `json:"permissions"`
	Roles       []ManifestRole       `json:"roles"`
	Routes      []ManifestRoute      `json:"routes"`
}

type ManifestPermission struct {
	Resource     string   `json:"resource"`
	Action       string   `json:"action"`
	Description  string   `json:"description"`
	Risk         string   `json:"risk"`
	MinAssurance int      `json:"min_assurance"`
	RequiresAMR  []string `json:"requires_amr"`
	MaxAuthAge   string   `json:"max_auth_age"`
}

type ManifestRole struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Permissions       []string `json:"permissions"` // "resource:action" within this app
	Patterns          []string `json:"patterns"`
	AllowedRealmKinds []string `json:"allowed_realm_kinds"`
}

type ManifestRoute struct {
	Priority      int               `json:"priority"`
	PathPattern   string            `json:"path_pattern"`
	HostPattern   string            `json:"host_pattern"`
	Methods       []string          `json:"methods"`
	Effect        string            `json:"effect"`
	Permission    string            `json:"permission"` // "resource:action"
	ScopeBindings map[string]string `json:"scope_bindings"`
}
