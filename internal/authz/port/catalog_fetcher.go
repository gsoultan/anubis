package authzport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
)

// CatalogFetcher reads a source and returns the document it holds, verbatim.
// Verbatim matters twice: the document's digest is what makes an unchanged
// catalog free, and a fetcher that "helpfully" reformats a file would make
// every poll look like a change.
type CatalogFetcher interface {
	Fetch(ctx context.Context, source catalogsync.Source) (document string, err error)
}
