package authzapp

import (
	"context"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
)

// ExplainUsecase returns the full evaluation tree. Not optional once more
// than two axes exist (docs/roadmap.md).
type ExplainUsecase interface {
	Execute(ctx context.Context, in AuthorizeInput) (*authzdomain.Explanation, error)
}
