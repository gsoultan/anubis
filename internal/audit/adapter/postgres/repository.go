package auditpg

import (
	"context"

	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rgen"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// Repository implements the audit context's ports over storm. It owns no
// connection: the shared database.DB decides whether a call runs on the pool
// or inside an ambient transaction, which is what keeps the chain append and
// its advisory lock in the same transaction without either knowing.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }

// ex binds storm to the right connection for this call.
func (s *Repository) ex(ctx context.Context) runtime.Executor { return s.StormExec(ctx) }
