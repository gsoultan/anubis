package usecase

import "context"

// DeviceVerifyUsecase completes device-key login: one ed25519.Verify over the
// consumed nonce.
type DeviceVerifyUsecase interface {
	Execute(ctx context.Context, in DeviceVerifyInput) (*TokenPair, error)
}
