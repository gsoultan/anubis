package scopeapp

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeAxisAdminUsecase interface {
	ListScopeAxes(ctx context.Context) ([]scopedomain.ScopeAxisRecord, error)
	// CreateScopeAxis registers a structure. With top set, its top level is
	// created in the same transaction — without one the structure can hold no
	// items until a level with no parents is added.
	CreateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord, top *scopedomain.ScopeNodeTypeRecord) (*scopedomain.ScopeAxisRecord, error)
	UpdateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) (*scopedomain.ScopeAxisRecord, error)
	// StrictDryRun replays recent allow decisions with the axis forced strict
	// and reports what would break. Run before flipping default_effect.
	StrictDryRun(ctx context.Context, axis string, sampleSize int) (sampled, wouldDeny int, examplesJSON string, err error)
}
