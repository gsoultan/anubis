// Package authzcatalog is what an application's catalog document IS: the
// permissions, roles and routes it declares, and the forms one arrives in.
// Applying a document is authzadmin's job — this package only decides what
// was said, so that a manifest posted to the API, a CSV an operator uploads
// and a CSV a scheduler pulls are the same document by the time anything
// touches the database.
package authzcatalog

// Manifest is the registration document applications ship
// (docs/api.md §Manifests). Applications own their catalogs; Anubis
// validates and stores, it does not curate.
type Manifest struct {
	Permissions []Permission `json:"permissions"`
	Roles       []Role       `json:"roles"`
	Routes      []Route      `json:"routes"`
}

type Permission struct {
	Resource     string   `json:"resource"`
	Action       string   `json:"action"`
	Description  string   `json:"description"`
	Risk         string   `json:"risk"`
	MinAssurance int      `json:"min_assurance"`
	RequiresAMR  []string `json:"requires_amr"`
	MaxAuthAge   string   `json:"max_auth_age"`
}

type Role struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Permissions       []string `json:"permissions"` // "resource:action" within this app
	Patterns          []string `json:"patterns"`
	AllowedRealmKinds []string `json:"allowed_realm_kinds"`
}

type Route struct {
	Priority      int               `json:"priority"`
	PathPattern   string            `json:"path_pattern"`
	HostPattern   string            `json:"host_pattern"`
	Methods       []string          `json:"methods"`
	Effect        string            `json:"effect"`
	Permission    string            `json:"permission"` // "resource:action"
	ScopeBindings map[string]string `json:"scope_bindings"`
}

// Document is one catalog document and — the part that decides how much
// damage it can do — WHICH SECTIONS IT ACTUALLY DECLARED.
//
// An absent section is not an empty section. A CSV of roles says nothing
// about permissions, and a CSV cannot express a route at all; read as
// "none", a roles upload would deprecate the whole permission catalog and
// delete every route policy the application has. So absent means untouched.
//
// Present-and-empty still means clear it: `"routes": []` is somebody saying
// so deliberately, which is a different act from not mentioning routes.
type Document struct {
	Permissions    []Permission
	HasPermissions bool
	Roles          []Role
	HasRoles       bool
	Routes         []Route
	HasRoutes      bool
}

// Sections names what the document declared, for the apply report. An
// operator reading "applied: roles" learns the permissions they expected to
// see were never in the file — the single most common import mistake.
func (d *Document) Sections() []string {
	out := make([]string, 0, 3)
	if d.HasPermissions {
		out = append(out, "permissions")
	}
	if d.HasRoles {
		out = append(out, "roles")
	}
	if d.HasRoutes {
		out = append(out, "routes")
	}
	return out
}
