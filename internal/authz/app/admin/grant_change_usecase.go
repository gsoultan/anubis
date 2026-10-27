package authzadmin

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grantchange"
	"github.com/gsoultan/anubis/internal/authz/domain/insight"
)

// GrantChangeUsecase is access looked at as a whole: what each person can
// do in a line, bulk changes to many grants as one undoable act, and the
// hand-picked lists that will quietly miss the next place.
type GrantChangeUsecase interface {
	SummarizeAccess(ctx context.Context, identityIDs []string) ([]insight.AccessSummary, error)
	PreviewGrantChange(ctx context.Context, s grantchange.Spec) (grantchange.Plan, error)
	ApplyGrantChange(ctx context.Context, s grantchange.Spec) (string, grantchange.Plan, error)
	UndoGrantChange(ctx context.Context, changeID, reason string) (int, error)
	ListGrantChanges(ctx context.Context, limit int) ([]grantchange.Record, error)
	ListFullLists(ctx context.Context, axis, roleID string) ([]insight.FullList, error)
	PlaceReach(ctx context.Context, nodeID string) (insight.PlaceReach, error)
	RoleUsage(ctx context.Context, roleIDs []string) ([]insight.RoleUsage, error)
	AppUsage(ctx context.Context, applicationID string) (insight.AppUsage, error)
	HolderKinds(ctx context.Context, roleID, axis string) (insight.HolderKinds, error)
	PlaceHolders(ctx context.Context, nodeID string, limit int) ([]insight.PlaceHolder, error)
}
