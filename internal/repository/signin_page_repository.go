package repository

import (
	"context"
	"time"
)

type SigninPageRepository interface {
	SigninPage(ctx context.Context, tenantID string) (config []byte, updatedAt time.Time, err error)
	PutSigninPage(ctx context.Context, tenantID string, config []byte) error
}
