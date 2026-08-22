package device

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
)

// DeviceVerifyUsecase completes device-key login: one ed25519.Verify over the
// consumed nonce.
type DeviceVerifyUsecase interface {
	Execute(ctx context.Context, in DeviceVerifyInput) (*authapp.TokenPair, error)
}
