package authzport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/insight"
)

// AccessInsightRepository reads access as a whole, for the console.
type AccessInsightRepository interface {
	SummarizeAccess(ctx context.Context, tenantID string, identityIDs []string) ([]insight.AccessSummary, error)
	ListFullLists(ctx context.Context, tenantID, axis, roleID string) ([]insight.FullList, error)
	PlaceReach(ctx context.Context, tenantID, nodeID string) (insight.PlaceReach, error)
}
