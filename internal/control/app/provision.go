package controlapp

import (
	"context"
	"fmt"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	controlport "github.com/gsoultan/anubis/internal/control/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
)

// ProvisionInput describes a tenant to bring into existence.
//
// Note what is NOT here. No admin role, no permission catalog, no
// applications: administering a tenant is a platform user's job
// (platform_assignments, ADR-0011), and a tenant's own roles and permissions
// belong to the applications its people use — which do not exist yet on a
// fresh tenant. The optional first user is an ordinary person for exercising
// tenant-facing flows; they hold no authority of any kind.
type ProvisionInput struct {
	TenantSlug string
	TenantName string

	FirstUsername string
	FirstEmail    string
	FirstPassword string
}

// ProvisionResult reports what was found and what was made, so a caller can
// tell "already set up" from "set up just now".
type ProvisionResult struct {
	TenantID      string
	RealmID       string
	FirstUserID   string
	TenantCreated bool
	UserCreated   bool
}

// Provision creates a usable tenant: the tenant row, its internal realm, and
// optionally one ordinary person.
//
// Every step looks before it creates, so running this twice is a no-op rather
// than an error — the installer and the bootstrap CLI both call it, and both
// get run again by people who are not sure whether the first attempt worked.
//
// The caller supplies the transaction; the whole tenant appears at once or
// not at all.
func Provision(
	ctx context.Context,
	in ProvisionInput,
	tenants controlport.TenantProvisioner,
	ids controlport.IdentityProvisioner,
) (*ProvisionResult, error) {
	res := &ProvisionResult{}

	tenant, err := tenants.TenantBySlug(ctx, in.TenantSlug)
	if err != nil {
		if tenant, err = tenants.CreateTenant(ctx, in.TenantSlug, in.TenantName); err != nil {
			return nil, fmt.Errorf("create tenant: %w", err)
		}
		res.TenantCreated = true
	}
	res.TenantID = tenant.ID

	realm, err := ids.RealmByCode(ctx, tenant.ID, "internal")
	if err != nil {
		id, cerr := ids.CreateRealm(ctx, tenant.ID, identitydomain.RealmRecord{
			Code: "internal", Kind: "internal", DisplayName: "Internal",
			MinAssurance:    1,
			AllowedFactors:  []string{"password", "totp", "device_key"},
			RequiredFactors: []string{"password"},
		})
		if cerr != nil {
			return nil, fmt.Errorf("create realm: %w", cerr)
		}
		if realm, err = ids.RealmByID(ctx, id); err != nil {
			return nil, err
		}
	}
	res.RealmID = realm.ID

	if in.FirstUsername == "" {
		return res, nil
	}
	// An ordinary person, nothing more: no grant, no role. Anything that
	// looks like handing authority out here would be the tenant-side admin
	// creeping back in.
	person, err := ids.IdentityForLogin(ctx, tenant.ID, realm.ID, in.FirstUsername)
	if err != nil || person == nil {
		hash, herr := kdf.Hash(in.FirstPassword)
		if herr != nil {
			return nil, herr
		}
		id, cerr := ids.CreateIdentity(ctx, identitydomain.IdentityCreate{
			TenantID: tenant.ID, RealmID: realm.ID, Username: in.FirstUsername,
			Email: in.FirstEmail, AssuranceLevel: realm.MinAssurance, Status: "active",
		})
		if cerr != nil {
			return nil, fmt.Errorf("create first user: %w", cerr)
		}
		if _, cerr := ids.CreateCredential(ctx, credential.CredentialInput{
			IdentityID: id, TenantID: tenant.ID, Kind: "password", Secret: hash,
		}); cerr != nil {
			return nil, cerr
		}
		res.FirstUserID = id
		res.UserCreated = true
		return res, nil
	}
	res.FirstUserID = person.ID
	return res, nil
}

// CreatePlatformOwner creates the installation owner: a platform user with
// authority over every tenant.
//
// It is NOT an identity. Whoever runs an installation is not a member of
// anything it hosts, so they live in their own table with no tenant, no realm
// and no grants (ADR-0011). Setup calls this once; it is a no-op afterwards,
// because an installation with two owners created by accident is worse than
// one that refused the second.
func CreatePlatformOwner(
	ctx context.Context,
	users controlport.PlatformUserStore,
	assign controlport.AssignmentWriter,
	username, email, password string,
) (string, error) {
	has, err := assign.HasOwner(ctx)
	if err != nil {
		return "", err
	}
	if has {
		return "", nil
	}
	hash, err := kdf.Hash(password)
	if err != nil {
		return "", err
	}
	id, err := users.CreatePlatformUser(ctx, username, email, hash)
	if err != nil {
		return "", fmt.Errorf("create platform owner: %w", err)
	}
	if _, err := assign.CreateAssignment(ctx, controldomain.AssignmentRecord{
		OperatorID: id,
		// No tenant means every tenant.
		Role:      controldomain.RoleOwner,
		GrantedBy: id,
		Reason:    "installation setup",
	}); err != nil {
		return "", err
	}
	return id, nil
}
