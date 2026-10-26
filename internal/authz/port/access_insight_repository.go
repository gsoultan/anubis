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
	RoleUsage(ctx context.Context, tenantID string, roleIDs []string) ([]insight.RoleUsage, error)
	AppUsage(ctx context.Context, tenantID, applicationID string) (insight.AppUsage, error)
	HolderKinds(ctx context.Context, tenantID, roleID, axis string) (insight.HolderKinds, error)
	PlaceHolders(ctx context.Context, tenantID, nodeID string, limit int) ([]insight.PlaceHolder, error)
}
