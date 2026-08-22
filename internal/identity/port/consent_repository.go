package identityport

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

type ConsentRepository interface {
	ListConsents(ctx context.Context, tenantID, identityID string) ([]identitydomain.ConsentRecord, error)
	InsertConsent(ctx context.Context, tenantID, identityID, purpose, policyVersion string, evidence []byte) (*identitydomain.ConsentRecord, error)
	WithdrawConsent(ctx context.Context, tenantID, id string) error
}
