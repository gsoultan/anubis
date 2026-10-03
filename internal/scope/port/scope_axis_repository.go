package scopeport

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

// ScopeAxisRepository is one tenant's structures (0056). Every method takes
// the tenant: a structure, and whether it is strict, used to be the
// installation's, so one tenant's operator could change every tenant's
// decisions.
type ScopeAxisRepository interface {
	ListScopeAxes(ctx context.Context, tenantID string) ([]scopedomain.ScopeAxisRecord, error)
	ScopeAxis(ctx context.Context, tenantID, code string) (*scopedomain.ScopeAxisRecord, error)
	CreateScopeAxis(ctx context.Context, tenantID string, a scopedomain.ScopeAxisRecord) error
	UpdateScopeAxis(ctx context.Context, tenantID string, a scopedomain.ScopeAxisRecord) error
}
