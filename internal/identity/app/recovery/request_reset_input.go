package identityrecovery

// RequestResetInput names the account the way a sign-in does: tenant slug,
// realm code (empty = "internal") and the username.
type RequestResetInput struct {
	Tenant   string
	Realm    string
	Username string
}
