package authzpg

import (
	"context"
	"fmt"
	_ "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// Repository implements the authz context's ports over storm: generated
// builder queries and the rquery declarations, both compiled against the
// schema at generate time. It owns no connection: the shared database.DB
// decides whether a call runs on the pool or inside an ambient transaction,
// and rex adapts that choice to storm's executor port per call.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }

// The executor and the id helpers every topical file in this package uses.
func (s *Repository) rex(ctx context.Context) runtime.Executor { return s.StormExec(ctx) }

func parseUUID(s string) ([16]byte, error) { return database.ParseUUID(s) }

func uuidStr(u [16]byte) string { return database.UUIDStr(u) }

func fmtSscanf(s, format string, args ...any) (int, error) {
	return fmt.Sscanf(s, format, args...)
}
