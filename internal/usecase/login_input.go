package usecase

// LoginInput carries the credentials presented at the front door.
type LoginInput struct {
	Tenant   string // tenant slug
	Realm    string // realm code; "" = internal
	Username string
	Password string
	ClientID string
	DeviceFP string
}
