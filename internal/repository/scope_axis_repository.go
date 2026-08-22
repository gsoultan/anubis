package repository

import "context"

type ScopeAxisRepository interface {
	ListScopeAxes(ctx context.Context) ([]ScopeAxisRecord, error)
	ScopeAxis(ctx context.Context, code string) (*ScopeAxisRecord, error)
	CreateScopeAxis(ctx context.Context, a ScopeAxisRecord) error
	UpdateScopeAxis(ctx context.Context, a ScopeAxisRecord) error
}
