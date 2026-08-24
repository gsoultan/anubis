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
	// IsSystem marks one of Anubis's own applications (ADR-0011). They own
	// the permission catalog and are never something a tenant's people sign
	// in to, so they stay out of the tenant's application list.
	IsSystem         bool
	ClientSecretHash string
}
