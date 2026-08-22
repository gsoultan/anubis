package repository

type IdentityFilter struct {
	RealmID string
	Status  string
	Query   string
	AfterID string
	Limit   int
}
