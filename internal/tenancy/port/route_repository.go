package tenancyport

import (
	"context"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

type RouteRepository interface {
	ListRoutePolicies(ctx context.Context, applicationID string) ([]tenancydomain.RoutePolicyRecord, error)
	ReplaceRoutePolicies(ctx context.Context, tenantID, applicationID string, policies []tenancydomain.RoutePolicyInput) error
}
