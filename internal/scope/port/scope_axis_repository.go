package scopeport

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeAxisRepository interface {
	ListScopeAxes(ctx context.Context) ([]scopedomain.ScopeAxisRecord, error)
	ScopeAxis(ctx context.Context, code string) (*scopedomain.ScopeAxisRecord, error)
	CreateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) error
	UpdateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) error
}
