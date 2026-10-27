package emaillink

// RequestInput asks for a sign-in link. Flow is the OIDC request the link
// must complete when opened — client_id, redirect_uri, state, code
// challenge, nonce — carried inside the one-time token's payload so the
// link itself is only a secret and nothing a mail client can rewrite.
type RequestInput struct {
	Tenant   string
	Realm    string
	Username string
	Flow     map[string]string
}
