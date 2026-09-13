package authzsvc

import (
	"context"

	authzcatalog "github.com/gsoultan/anubis/internal/authz/app/catalog"
	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
)

// CatalogAdminService is the OPERATOR-facing half of catalog sync.
//
// RunDue is deliberately absent. The scheduler's entry point checks no
// permission and takes its own tenant, which is correct for a timer and
// indefensible on a surface a request can reach — so the transport is handed
// a type that does not have the method at all, not merely a rule saying not
// to call it.
type CatalogAdminService interface {
	ListSources(ctx context.Context) ([]catalogsync.Source, error)
	CreateSource(ctx context.Context, in authzcatalog.SourceInput) (*catalogsync.Source, error)
	UpdateSource(ctx context.Context, in authzcatalog.SourceInput) (*catalogsync.Source, error)
	DeleteSource(ctx context.Context, sourceID string) error
	RunSource(ctx context.Context, sourceID string, dry bool) (*catalogsync.Run, error)
	ListRuns(ctx context.Context, sourceID string, limit int32) ([]catalogsync.Run, error)
}

// NewCatalogAdminService narrows the sync usecase to what a transport may do.
func NewCatalogAdminService(u authzcatalog.CatalogSyncUsecase) CatalogAdminService { return u }
