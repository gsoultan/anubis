package authpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) SweepExpired(ctx context.Context) (int64, error) {
	n, err := s.q(ctx).SweepOneTimeTokens(ctx)
	return n, database.MapErr(err)
}
