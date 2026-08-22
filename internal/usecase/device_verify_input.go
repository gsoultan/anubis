package usecase

type DeviceVerifyInput struct {
	Tenant    string
	Nonce     string
	KeyID     string // credential id
	Signature string // base64url over the raw nonce bytes
	ClientID  string
	DeviceFP  string
}
