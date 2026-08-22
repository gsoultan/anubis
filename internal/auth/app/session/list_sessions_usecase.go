package sessionapp

import (
	"context"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

// ListSessionsUsecase is the user-visible device list.
type ListSessionsUsecase interface {
	Execute(ctx context.Context) ([]authdomain.SessionInfo, string, error)
}
