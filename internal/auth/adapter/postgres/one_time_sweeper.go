package authpg

import (
	"context"

	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// SweepExpired clears one-time tokens an hour past expiry.
//
// An hour late rather than immediately: a just-expired row still answers "was
// this presented too late?" for as long as somebody might present it.
func (s *Repository) SweepExpired(ctx context.Context) (int64, error) {
	n, err := authrquery.SweepOneTimeTokens.Exec(ctx, s.ex(ctx))
	return n, database.MapErr(err)
}
