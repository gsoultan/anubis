package authpg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/auth/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// Repository implements the auth context's ports over its own generated
// query package. It owns no connection: the shared database.DB decides
// whether a call runs on the pool or inside an ambient transaction.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }

// q binds the generated queries to the right connection for this call.
func (s *Repository) q(ctx context.Context) *gen.Queries {
	return gen.New(s.Conn(ctx))
}
