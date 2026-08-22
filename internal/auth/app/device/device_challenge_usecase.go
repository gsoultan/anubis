package device

import "context"

// DeviceChallengeUsecase begins biometric device-key login: a single-use
// nonce the device signs after a LOCAL biometric unlock. Biometric data
// never reaches the server.
type DeviceChallengeUsecase interface {
	Execute(ctx context.Context, in DeviceChallengeInput) (*DeviceChallengeOutput, error)
}
