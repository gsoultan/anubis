package accesslink

// OpenInput presents an access link. Tenant and ClientID name where it is
// being opened: the token minted is for that application only.
type OpenInput struct {
	Tenant   string
	ClientID string
	Token    string
}
