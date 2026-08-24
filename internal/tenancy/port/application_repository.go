package tenancyport

import (
	"context"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

type ApplicationRepository interface {
	ApplicationBySlug(ctx context.Context, tenantID, slug string) (*tenancydomain.ApplicationRecord, error)
	ApplicationByID(ctx context.Context, tenantID, id string) (*tenancydomain.ApplicationRecord, error)
	ListApplications(ctx context.Context, tenantID, query, after string, pageSize int32) ([]tenancydomain.ApplicationRecord, error)
	CountApplications(ctx context.Context, tenantID string) (int, error)
	AllApplications(ctx context.Context, tenantID string) ([]tenancydomain.ApplicationRecord, error)
	CreateApplication(ctx context.Context, tenantID string, a tenancydomain.ApplicationRecord) (string, error)
	UpdateApplication(ctx context.Context, tenantID string, a tenancydomain.ApplicationRecord) error
	SetClientSecretHash(ctx context.Context, tenantID, id, hash string) error
	BumpManifestVersion(ctx context.Context, applicationID string) (int, error)
}
