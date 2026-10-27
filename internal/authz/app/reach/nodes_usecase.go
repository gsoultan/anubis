package authzreach

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/granted"
)

// NodesUsecase lists the places on one axis where a subject holds a
// permission. Served from the gate's in-memory snapshot rather than the SQL
// engine: it is asked once per screen by every application that filters a
// list, and a question asked that often belongs on the path built for
// volume. The gate context implements it; this is the authz context's view.
type NodesUsecase interface {
	Execute(ctx context.Context, in NodesInput) (*granted.Nodes, error)
}
