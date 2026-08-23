package tenancydomain

// AuthPageInput is a create or update from the builder. Config is validated
// by pagecfg before it reaches the repository — an unvalidated config is a
// stylesheet injection waiting to be served on the password screen.
type AuthPageInput struct {
	ID            string
	Kind          string
	Slug          string
	Name          string
	Status        string
	ApplicationID string
	Config        []byte
}
