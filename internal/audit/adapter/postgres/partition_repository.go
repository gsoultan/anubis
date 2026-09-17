package auditpg

import (
	"context"

	auditrquery "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// EnsurePartitions provisions three months ahead for both partitioned tables.
//
// Running ahead rather than on demand is the point: a write that arrives with
// no partition to land in fails, and the month boundary is exactly when
// nobody is watching.
func (s *Repository) EnsurePartitions(ctx context.Context) error {
	if _, _, err := auditrquery.EnsureAuditPartitions.One(ctx, s.ex(ctx)); err != nil {
		return database.MapErr(err)
	}
	_, _, err := auditrquery.EnsureRefreshPartitions.One(ctx, s.ex(ctx))
	return database.MapErr(err)
}
