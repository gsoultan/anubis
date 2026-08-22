package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type ScopeAxisAdminUsecase interface {
	ListScopeAxes(ctx context.Context) ([]repository.ScopeAxisRecord, error)
	CreateScopeAxis(ctx context.Context, a repository.ScopeAxisRecord) (*repository.ScopeAxisRecord, error)
	UpdateScopeAxis(ctx context.Context, a repository.ScopeAxisRecord) (*repository.ScopeAxisRecord, error)
	// StrictDryRun replays recent allow decisions with the axis forced strict
	// and reports what would break. Run before flipping default_effect.
	StrictDryRun(ctx context.Context, axis string, sampleSize int) (sampled, wouldDeny int, examplesJSON string, err error)
}
