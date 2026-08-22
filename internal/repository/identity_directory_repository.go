package repository

import "context"

type IdentityDirectoryRepository interface {
	ListIdentities(ctx context.Context, tenantID string, f IdentityFilter) ([]IdentityRecord, error)
	IdentityRecordByID(ctx context.Context, tenantID, id string) (*IdentityRecord, error)
}
