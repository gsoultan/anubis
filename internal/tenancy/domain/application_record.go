package tenancydomain

type ApplicationRecord struct {
	ID                     string
	Slug                   string
	Name                   string
	Kind                   string
	Status                 string
	RedirectURIs           []string
	PostLogoutRedirectURIs []string
	BackchannelLogoutURI   string
	TokenFormat            string
	AccessTokenTTL         string
	RefreshTokenTTL        string
	AccessTokenTTLSecs     int64
	RefreshTokenTTLSecs    int64
	ManifestVersion        int
	ClientSecretHash       string
}
