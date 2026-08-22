package repository

import (
	"context"
	"github.com/gsoultan/anubis/internal/domain"
)

// RealmRepository resolves population policy.
type RealmRepository interface {
	RealmByCode(ctx context.Context, tenantID, code string) (*domain.Realm, error)
	RealmByID(ctx context.Context, id string) (*domain.Realm, error)
}
