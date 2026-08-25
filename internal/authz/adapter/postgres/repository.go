package authzpg

import (
	"github.com/gsoultan/anubis/internal/platform/database"
)

// Repository implements the authz context's ports over raorm: generated
// builder queries and the rquery declarations, both compiled against the
// schema at generate time. It owns no connection: the shared database.DB
// decides whether a call runs on the pool or inside an ambient transaction,
// and rex adapts that choice to raorm's executor port per call.
type Repository struct {
	*database.DB
}

func New(db *database.DB) *Repository { return &Repository{DB: db} }
