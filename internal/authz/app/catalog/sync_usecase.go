package authzcatalog

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
)

// SourceInput is what an operator configures. The application is named by
// slug because that is what an operator knows; it is resolved to an id once
// and the id is the pin from then on.
type SourceInput struct {
	ID              string
	ApplicationSlug string
	Name            string
	Kind            string
	Format          string
	Status          string
	ConfigJSON      string
	IntervalSeconds int
}

// CatalogSyncUsecase is the catalog's fourth channel: sources Anubis reads
// on its own. The other three — an API call, an uploaded file, a hand-edited
// role — all land in ApplyManifest and need nothing here.
type CatalogSyncUsecase interface {
	ListSources(ctx context.Context) ([]catalogsync.Source, error)
	CreateSource(ctx context.Context, in SourceInput) (*catalogsync.Source, error)
	UpdateSource(ctx context.Context, in SourceInput) (*catalogsync.Source, error)
	// RunSource is the button an operator presses. Dry proves the document
	// parses and reports the diff without writing.
	RunSource(ctx context.Context, sourceID string, dry bool) (*catalogsync.Run, error)
	DeleteSource(ctx context.Context, sourceID string) error
	ListRuns(ctx context.Context, sourceID string, limit int32) ([]catalogsync.Run, error)
	// RunDue is the scheduler's entry point and is NOT operator-facing: it
	// crosses every tenant and runs as 'system'. No transport may call it —
	// scripts/check/import-boundary.sh enforces that.
	RunDue(ctx context.Context, now time.Time, limit int32) (ran int, err error)
}

// CatalogApplier is the narrow slice of the authz admin usecase this needs,
// declared here and satisfied structurally by the object the composition
// root already builds — the same arrangement the provisioning context uses.
type CatalogApplier interface {
	ApplyDocumentAsSystem(ctx context.Context, tenantID, applicationSlug, document, format string, dry bool) (reportJSON string, version int, err error)
}
