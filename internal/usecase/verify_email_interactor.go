package usecase

import (
	"context"
	"encoding/json"

	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// verifyEmailInteractor implements VerifyEmailUsecase.
type verifyEmailInteractor struct {
	onetime repository.OneTimeRepository
	ids     repository.IdentityRepository
}

func NewVerifyEmailInteractor(onetime repository.OneTimeRepository, ids repository.IdentityRepository) VerifyEmailUsecase {
	return &verifyEmailInteractor{onetime: onetime, ids: ids}
}

func (u *verifyEmailInteractor) Execute(ctx context.Context, token string) error {
	if token == "" || len(token) > 128 {
		return domain.ErrTokenInvalid
	}
	_, payload, err := u.onetime.ConsumeOneTime(ctx, "email_verify", secret.Hash(token))
	if err != nil {
		return domain.ErrTokenInvalid
	}
	var p struct {
		IdentityID string `json:"identity_id"`
		TenantID   string `json:"tenant_id"`
	}
	if json.Unmarshal(payload, &p) != nil || p.IdentityID == "" {
		return domain.ErrTokenInvalid
	}
	// pending -> active
	return u.ids.EnableIdentity(ctx, p.TenantID, p.IdentityID)
}
