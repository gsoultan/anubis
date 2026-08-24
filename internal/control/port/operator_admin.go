package controlport

import (
	"context"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// PlatformUserStore holds the people who operate this installation. It is a
// different table from identities on purpose (ADR-0011): nothing joins the
// two, so a tenant's user cannot become an operator by any route.
type PlatformUserStore interface {
	CreatePlatformUser(ctx context.Context, username, email, passwordHash string) (string, error)
	// PlatformUserByUsername returns the user and their password hash, or a
	// nil user when nobody holds that name.
	PlatformUserByUsername(ctx context.Context, username string) (*controldomain.PlatformUser, string, error)
	// ListPlatformUsers returns one keyset page ordered by username; after is
	// the last username of the previous page, or "" for the first.
	ListPlatformUsers(ctx context.Context, query, after string, pageSize int32) ([]controldomain.PlatformUser, error)
	CountPlatformUsers(ctx context.Context) (int, error)
	AnyPlatformUser(ctx context.Context) (bool, error)
	// TouchLogin records a successful sign-in. Best effort: failing to note
	// the time is not a reason to refuse somebody entry.
	TouchLogin(ctx context.Context, id string)
	SetStatus(ctx context.Context, id, status string) error
	PlatformUserByID(ctx context.Context, id string) (*controldomain.PlatformUser, string, error)
	TOTPSecret(ctx context.Context, master []byte, id string) ([]byte, error)
	StageTOTPSecret(ctx context.Context, master []byte, id string, secret []byte) error
	ConfirmTOTP(ctx context.Context, id string, step uint64) error
	AdvanceTOTPStep(ctx context.Context, id string, step uint64) (bool, error)
	ClearTOTP(ctx context.Context, id string) error
}

// TenantLookup resolves the tenants an operator can be assigned to.
type TenantLookup interface {
	TenantBySlug(ctx context.Context, slug string) (*tenancydomain.TenantRef, error)
	// ListTenants backs the header picker for an installation owner, whose
	// authority covers every tenant rather than a listed few.
	ListTenants(ctx context.Context) ([]tenancydomain.TenantRef, error)
}

// AssignmentReader is the user-management page's read side.
type AssignmentReader interface {
	Assignments(ctx context.Context) ([]controldomain.AssignmentRecord, error)
	RevokeAssignment(ctx context.Context, id string) error
}
