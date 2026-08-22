package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

// ListSessionsUsecase is the user-visible device list.
type ListSessionsUsecase interface {
	Execute(ctx context.Context) ([]repository.SessionInfo, string, error)
}
