package identityapp

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

// ConsentAdminUsecase records lawful-basis consents. Withdrawal appends;
// the record of what was consented to survives (docs/security.md).
type ConsentAdminUsecase interface {
	ListConsents(ctx context.Context, identityID string) ([]identitydomain.ConsentRecord, error)
	RecordConsent(ctx context.Context, identityID, purpose, policyVersion, evidenceJSON string) (*identitydomain.ConsentRecord, error)
	WithdrawConsent(ctx context.Context, consentID string) error
}
