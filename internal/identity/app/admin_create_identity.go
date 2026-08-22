package identityapp

// AdminCreateIdentity is the admin-plane identity creation input.
type AdminCreateIdentity struct {
	Realm          string
	Username       string
	Email          string
	Password       string
	Category       string
	ExternalRef    string
	AssuranceLevel int
}
