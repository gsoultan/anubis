package tenancyport

import (
	"context"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// AuthPageRepository stores the sign-in and sign-out pages a tenant serves.
type AuthPageRepository interface {
	ListAuthPages(ctx context.Context, tenantID, kind string) ([]tenancydomain.AuthPage, error)
	AuthPage(ctx context.Context, tenantID, id string) (*tenancydomain.AuthPage, error)
	AuthPageBySlug(ctx context.Context, tenantID, kind, slug string) (*tenancydomain.AuthPage, error)
	DefaultAuthPage(ctx context.Context, tenantID, kind string) (*tenancydomain.AuthPage, error)
	AuthPageForApplication(ctx context.Context, tenantID, kind, applicationID string) (*tenancydomain.AuthPage, error)
	// AuthPageForRealm is the population's own door — the page internal,
	// partner or public users see. Resolution tries it AFTER the application
	// binding and BEFORE the tenant default, so an application that
	// configured its own page keeps it (migration 0041).
	AuthPageForRealm(ctx context.Context, tenantID, kind, realmID string) (*tenancydomain.AuthPage, error)
	CreateAuthPage(ctx context.Context, tenantID string, in tenancydomain.AuthPageInput) (string, error)
	UpdateAuthPage(ctx context.Context, tenantID string, in tenancydomain.AuthPageInput) error
	DeleteAuthPage(ctx context.Context, tenantID, id string) error
	// SetDefaultAuthPage promotes one page and demotes the previous default in
	// the same transaction; the schema permits only one at a time.
	SetDefaultAuthPage(ctx context.Context, tenantID, kind, id string) error
}
