package registration

import (
	"context"
	"encoding/json"

	authport "github.com/gsoultan/anubis/internal/auth/port"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// verifyEmailInteractor implements VerifyEmailUsecase.
type verifyEmailInteractor struct {
	onetime authport.OneTimeRepository
	ids     identityport.IdentityRepository
}

func NewVerifyEmailInteractor(onetime authport.OneTimeRepository, ids identityport.IdentityRepository) VerifyEmailUsecase {
	return &verifyEmailInteractor{onetime: onetime, ids: ids}
}

func (u *verifyEmailInteractor) Execute(ctx context.Context, token string) error {
	if token == "" || len(token) > 128 {
		return apperr.ErrTokenInvalid
	}
	_, payload, err := u.onetime.ConsumeOneTime(ctx, "email_verify", secret.Hash(token))
	if err != nil {
		return apperr.ErrTokenInvalid
	}
	var p struct {
		IdentityID string `json:"identity_id"`
		TenantID   string `json:"tenant_id"`
	}
	if json.Unmarshal(payload, &p) != nil || p.IdentityID == "" {
		return apperr.ErrTokenInvalid
	}
	// pending -> active
	return u.ids.EnableIdentity(ctx, p.TenantID, p.IdentityID)
}
