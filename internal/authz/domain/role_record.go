package authzdomain

type RoleRecord struct {
	ID                string
	Name              string
	Description       string
	ApplicationSlug   string
	IsSystem          bool
	AllowedRealmKinds []string
	AssignableAt      []string
}
