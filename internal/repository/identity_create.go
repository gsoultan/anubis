package repository

type IdentityCreate struct {
	TenantID       string
	RealmID        string
	Username       string
	Email          string
	ExternalRef    string
	AssuranceLevel int
	CategoryID     string
	Status         string
}
