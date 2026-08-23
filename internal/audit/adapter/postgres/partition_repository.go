package auditpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
)

// EnsurePartitions provisions three months ahead for both partitioned tables.
func (s *Repository) EnsurePartitions(ctx context.Context) error {
	if err := s.q(ctx).EnsureAuditPartitions(ctx); err != nil {
		return database.MapErr(err)
	}
	return database.MapErr(s.q(ctx).EnsureRefreshPartitions(ctx))
}
