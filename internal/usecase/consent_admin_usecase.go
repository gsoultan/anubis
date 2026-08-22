package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

// ConsentAdminUsecase records lawful-basis consents. Withdrawal appends;
// the record of what was consented to survives (docs/security.md).
type ConsentAdminUsecase interface {
	ListConsents(ctx context.Context, identityID string) ([]repository.ConsentRecord, error)
	RecordConsent(ctx context.Context, identityID, purpose, policyVersion, evidenceJSON string) (*repository.ConsentRecord, error)
	WithdrawConsent(ctx context.Context, consentID string) error
}
