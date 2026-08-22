package repository

import "context"

type ApplicationRepository interface {
	ApplicationBySlug(ctx context.Context, tenantID, slug string) (*ApplicationRecord, error)
	ApplicationByID(ctx context.Context, tenantID, id string) (*ApplicationRecord, error)
	ListApplications(ctx context.Context, tenantID string) ([]ApplicationRecord, error)
	CreateApplication(ctx context.Context, tenantID string, a ApplicationRecord) (string, error)
	UpdateApplication(ctx context.Context, tenantID string, a ApplicationRecord) error
	SetClientSecretHash(ctx context.Context, tenantID, id, hash string) error
	BumpManifestVersion(ctx context.Context, applicationID string) (int, error)
}
