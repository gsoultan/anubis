package authzdomain

type RoleRecord struct {
	ID                string
	Name              string
	Description       string
	ApplicationSlug   string
	IsSystem          bool
	AllowedRealmKinds []string
	AssignableAt      []string
	// Deprecated: retired from the catalog. It cannot be granted to anybody
	// new; every grant that already names it keeps working, because
	// authorize() does not look at this (migration 0044).
	Deprecated bool
}
