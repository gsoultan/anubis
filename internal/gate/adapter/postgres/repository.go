package gatepg

import (
	"context"

	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/gate/adapter/postgres/rgen"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// Repository implements the gate context's ports over storm. It owns no
// connection and no TABLE: the gate reads eight of other contexts' to freeze
// a snapshot, which is why this adapter declares no model.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }

// ex binds storm to the right connection for this call. The snapshot load
// does NOT use it — that one binds to its own repeatable-read transaction.
func (s *Repository) ex(ctx context.Context) runtime.Executor { return s.StormExec(ctx) }

// nstr turns storm's nullable text into the plain string the snapshot uses.
func nstr(n runtime.Null[string]) string {
	v, _ := n.Get()
	return v
}
