package registration

type RegisterConsent struct {
	Purpose       string
	PolicyVersion string
}

type RegisterInput struct {
	Tenant   string
	Realm    string
	Username string
	Email    string
	Password string
	Consents []RegisterConsent
}
