package tokenapp

// ClientCredentialsInput is the presented client identity.
type ClientCredentialsInput struct {
	Tenant       string
	ClientID     string
	ClientSecret string
	Audience     string
}
