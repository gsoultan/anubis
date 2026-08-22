package repository

import "context"

type PermissionCatalogRepository interface {
	ListPermissions(ctx context.Context, tenantID, applicationID string, includeDeprecated bool) ([]PermissionRecord, error)
	UpsertPermission(ctx context.Context, tenantID, applicationID, appSlug string, p PermissionRecord) (id string, key string, err error)
	DeprecatePermissionsExcept(ctx context.Context, applicationID string, keepIDs []string) ([]string, error)
	PermissionIDByKey(ctx context.Context, tenantID, key string) (string, error)
}
