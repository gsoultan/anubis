// Package controlport is what the control plane needs from the rest of the
// system, stated as the narrowest interfaces that will do.
//
// Each is satisfied structurally by a context's own repository, so the
// installer and the bootstrap CLI provision a tenant through ports and domain
// types rather than by reaching into another context's adapter.
package controlport

import (
	"context"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// TenantProvisioner creates the tenant an installation is built around.
type TenantProvisioner interface {
	TenantBySlug(ctx context.Context, slug string) (*tenancydomain.TenantRef, error)
	CreateTenant(ctx context.Context, slug, name string) (*tenancydomain.TenantRef, error)
}

// IdentityProvisioner creates the realm a tenant's people live in, and
// optionally the first of them. Never an administrator: administering a
// tenant is a platform user's job, decided by platform_assignments.
type IdentityProvisioner interface {
	RealmByCode(ctx context.Context, tenantID, code string) (*identitydomain.Realm, error)
	RealmByID(ctx context.Context, id string) (*identitydomain.Realm, error)
	CreateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) (string, error)
	IdentityForLogin(ctx context.Context, tenantID, realmID, username string) (*identitydomain.Identity, error)
	CreateIdentity(ctx context.Context, in identitydomain.IdentityCreate) (string, error)
	CreateCredential(ctx context.Context, in credential.CredentialInput) (string, error)
}

// AssignmentWriter records control-plane authority.
type AssignmentWriter interface {
	CreateAssignment(ctx context.Context, a controldomain.AssignmentRecord) (string, error)
	HasOwner(ctx context.Context) (bool, error)
}
