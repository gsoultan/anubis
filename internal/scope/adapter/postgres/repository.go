package scopepg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/scope/adapter/postgres/rgen"
	"github.com/gsoultan/storm/runtime"
)

// Repository implements the scope context's ports over storm. It owns no
// connection: the shared database.DB decides whether a call runs on the pool
// or inside an ambient transaction.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }

// ex binds storm to the right connection for this call.
func (s *Repository) ex(ctx context.Context) runtime.Executor { return s.StormExec(ctx) }

// nstr turns storm's nullable text into the plain string the domain uses.
func nstr(n runtime.Null[string]) string {
	v, _ := n.Get()
	return v
}
