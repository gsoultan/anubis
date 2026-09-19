package authpg

import (
	"context"
	"time"

	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rgen"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// Repository implements the auth context's ports over storm. It owns no
// connection: the shared database.DB decides whether a call runs on the pool
// or inside an ambient transaction — which is what keeps a rotation's claim
// and its successor in one transaction without either knowing.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }

// ex binds storm to the right connection for this call.
func (s *Repository) ex(ctx context.Context) runtime.Executor { return s.StormExec(ctx) }

// tptr turns storm's nullable timestamp into the *time.Time the domain uses.
func tptr(n runtime.Null[time.Time]) *time.Time {
	if v, ok := n.Get(); ok {
		return &v
	}
	return nil
}

// nstr turns storm's nullable text into the plain string the domain uses.
func nstr(n runtime.Null[string]) string {
	v, _ := n.Get()
	return v
}
