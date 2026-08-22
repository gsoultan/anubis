package authzadmin

import "context"

type ManifestAdminUsecase interface {
	// ApplyManifest validates, diffs and applies an application's permission/
	// role/route catalog. Removed permissions are deprecated, never deleted.
	ApplyManifest(ctx context.Context, applicationSlug, manifestJSON string, dry bool) (reportJSON string, manifestVersion int, err error)
}
