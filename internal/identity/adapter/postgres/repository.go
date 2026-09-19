package identitypg

import (
	"context"
	"time"

	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rgen"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// Repository implements the identity context's ports over storm. It owns no
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

// tptr turns storm's nullable timestamp into the *time.Time the domain uses.
func tptr(n runtime.Null[time.Time]) *time.Time {
	if v, ok := n.Get(); ok {
		return &v
	}
	return nil
}

// present reports whether a nullable timestamp is set, which is how the domain
// asks "is this disabled / anonymised".
func present[T any](n runtime.Null[T]) bool {
	_, ok := n.Get()
	return ok
}

// optArg turns an absent id or filter into SQL NULL. ” would be a value the
// predicate could never match, and for a uuid column PostgreSQL rejects it
// outright — which is how a tenant-wide listing used to 500.
func optArg(v string) any {
	if v == "" {
		return nil
	}
	return v
}
