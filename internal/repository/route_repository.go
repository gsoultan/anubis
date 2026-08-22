package repository

import "context"

type RouteRepository interface {
	ListRoutePolicies(ctx context.Context, applicationID string) ([]RoutePolicyRecord, error)
	ReplaceRoutePolicies(ctx context.Context, tenantID, applicationID string, policies []RoutePolicyInput) error
}
