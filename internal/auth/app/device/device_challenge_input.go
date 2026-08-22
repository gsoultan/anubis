package device

type DeviceChallengeInput struct {
	Tenant   string
	Realm    string
	DeviceID string // credential id of the enrolled device key
}
