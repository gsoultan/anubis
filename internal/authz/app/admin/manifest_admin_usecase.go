package authzadmin

import "context"

type ManifestAdminUsecase interface {
	// ApplyManifest validates, diffs and applies an application's permission/
	// role/route catalog. Removed permissions are deprecated, never deleted.
	//
	// document is JSON or CSV, named by format (empty means JSON). Only the
	// sections the document declares are touched: a CSV of roles cannot
	// express a permission or a route, and must not be read as declaring
	// that the application has none.
	ApplyManifest(ctx context.Context, applicationSlug, document, format string, dry bool) (reportJSON string, manifestVersion int, err error)

	// ApplyDocumentAsSystem is the same apply with NO operator check, for the
	// catalog scheduler — a timer has no operator to check, and the authority
	// for the run was settled when the source was configured.
	//
	// NO TRANSPORT MAY CALL THIS. A caller that supplies its own tenant id is
	// a caller that has not proved it may touch that tenant, and
	// scripts/check/import-boundary.sh fails the build if an adapter names it.
	ApplyDocumentAsSystem(ctx context.Context, tenantID, applicationSlug, document, format string, dry bool) (reportJSON string, manifestVersion int, err error)
}
