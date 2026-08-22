package usecase

import "context"

// ExplainUsecase returns the full evaluation tree. Not optional once more
// than two axes exist (docs/roadmap.md).
type ExplainUsecase interface {
	Execute(ctx context.Context, in AuthorizeInput) (*Explanation, error)
}
