package tenancyapp

import (
	"context"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// PageAdminUsecase is the sign-in / sign-out page builder's write side.
// Config is validated here, not at the transport: a page reaches the database
// only if it is renderable.
type PageAdminUsecase interface {
	ListAuthPages(ctx context.Context, kind string) ([]tenancydomain.AuthPage, error)
	GetAuthPage(ctx context.Context, id string) (*tenancydomain.AuthPage, error)
	CreateAuthPage(ctx context.Context, in tenancydomain.AuthPageInput) (*tenancydomain.AuthPage, error)
	UpdateAuthPage(ctx context.Context, in tenancydomain.AuthPageInput) (*tenancydomain.AuthPage, error)
	DeleteAuthPage(ctx context.Context, id string) error
	SetDefaultAuthPage(ctx context.Context, id string) error
	// PreviewAuthPage validates a config without storing it, so the builder
	// can show the same errors the save would produce.
	PreviewAuthPage(ctx context.Context, kind string, config []byte) error
}
