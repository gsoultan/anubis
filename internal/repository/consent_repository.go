package repository

import "context"

type ConsentRepository interface {
	ListConsents(ctx context.Context, tenantID, identityID string) ([]ConsentRecord, error)
	InsertConsent(ctx context.Context, tenantID, identityID, purpose, policyVersion string, evidence []byte) (*ConsentRecord, error)
	WithdrawConsent(ctx context.Context, tenantID, id string) error
}
