package authzpg

import (
	"context"

	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// rex adapts the ambient connection to storm's executor port.
//
// The adapter itself lives in platform/database: it is plumbing every storm
// context needs, and a copy per context is a copy per context to forget when
// database.Conn grows a third producer. These one-line wrappers keep the call
// sites in this package reading as they did.
func (s *Repository) rex(ctx context.Context) runtime.Executor { return s.StormExec(ctx) }

func parseUUID(s string) ([16]byte, error) { return database.ParseUUID(s) }

func uuidStr(u [16]byte) string { return database.UUIDStr(u) }
