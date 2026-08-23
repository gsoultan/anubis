package clientcreds

import "context"

// ClientCredentialsUsecase implements the OAuth2 client_credentials grant for
// service-to-service calls. A service does not "log in": there is no session,
// no refresh token and no user — just a short-lived access token proving
// which application is calling.
type ClientCredentialsUsecase interface {
	Execute(ctx context.Context, in ClientCredentialsInput) (*ClientCredentialsOutput, error)
}
