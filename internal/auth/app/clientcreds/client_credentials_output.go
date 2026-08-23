package clientcreds

// ClientCredentialsOutput is a bare access token: no refresh, no session.
type ClientCredentialsOutput struct {
	AccessToken string
	TokenType   string
	ExpiresIn   int
}
