package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/anubis/internal/config"
	"github.com/gsoultan/anubis/internal/crypto/kdf"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/repository/postgres"
)

// selfPermissions is Anubis's own catalog — Anubis is its own relying party.
// Delegated administration works through ordinary grants of these.
var selfPermissions = []struct{ resource, action, description, risk string }{
	{"identity", "read", "Read identities and their credentials", "normal"},
	{"identity", "write", "Create, modify, disable identities", "sensitive"},
	{"credential", "write", "Issue and revoke credentials and API keys", "sensitive"},
	{"consent", "write", "Record and withdraw consents", "normal"},
	{"realm", "admin", "Manage realms and categories", "critical"},
	{"application", "admin", "Manage applications and secrets", "critical"},
	{"scope", "admin", "Manage scope axes and nodes", "sensitive"},
	{"role", "admin", "Manage roles and their permissions", "critical"},
	{"grant", "admin", "Create and revoke grants", "critical"},
	{"membership", "admin", "Manage memberships", "sensitive"},
	{"manifest", "apply", "Apply application manifests", "sensitive"},
	{"audit", "read", "Query the audit log", "sensitive"},
	{"key", "admin", "Manage signing keys", "critical"},
	{"signin", "admin", "Manage the sign-in page", "normal"},
	{"tenant", "admin", "Manage tenants", "critical"},
	{"sync", "admin", "Manage scope sync sources", "sensitive"},
}

// runBootstrap provisions a usable installation: tenant, internal realm,
// admin identity, console + anubis applications, the anubis.admin role
// (pattern anubis:*) and the admin's grant.
func runBootstrap(ctx context.Context, logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	tenantSlug := fs.String("tenant", "impack", "tenant slug")
	tenantName := fs.String("name", "Impack", "tenant display name")
	adminUser := fs.String("admin-user", "admin", "admin username")
	adminPass := fs.String("admin-pass", "", "admin password (required)")
	consoleOrigin := fs.String("console-origin", "http://localhost:7447", "console origin for redirect URIs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *adminPass == "" {
		return fmt.Errorf("--admin-pass is required")
	}
	if len(*adminPass) < 12 {
		return fmt.Errorf("--admin-pass must be at least 12 characters")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := postgres.NewStore(pool)

	return store.WithinTx(ctx, func(ctx context.Context) error {
		tenant, err := store.TenantBySlug(ctx, *tenantSlug)
		if err != nil {
			if tenant, err = store.CreateTenant(ctx, *tenantSlug, *tenantName); err != nil {
				return fmt.Errorf("create tenant: %w", err)
			}
			logger.Info("tenant created", "slug", *tenantSlug)
		}

		realm, err := store.RealmByCode(ctx, tenant.ID, "internal")
		if err != nil {
			id, cerr := store.CreateRealm(ctx, tenant.ID, repository.RealmRecord{
				Code: "internal", Kind: "internal", DisplayName: "Internal",
				MinAssurance:    1,
				AllowedFactors:  []string{"password", "totp", "device_key"},
				RequiredFactors: []string{"password"},
			})
			if cerr != nil {
				return fmt.Errorf("create realm: %w", cerr)
			}
			if realm, err = store.RealmByID(ctx, id); err != nil {
				return err
			}
			logger.Info("internal realm created")
		}

		// Applications: the console (browser SPA) and anubis itself.
		if _, err := store.ApplicationBySlug(ctx, tenant.ID, "console"); err != nil {
			if _, err := store.CreateApplication(ctx, tenant.ID, repository.ApplicationRecord{
				Slug: "console", Name: "Anubis Console", Kind: "spa",
				RedirectURIs: []string{*consoleOrigin + "/callback"},
			}); err != nil {
				return fmt.Errorf("create console app: %w", err)
			}
		}
		anubisApp, err := store.ApplicationBySlug(ctx, tenant.ID, "anubis")
		if err != nil {
			id, cerr := store.CreateApplication(ctx, tenant.ID, repository.ApplicationRecord{
				Slug: "anubis", Name: "Anubis", Kind: "service",
			})
			if cerr != nil {
				return fmt.Errorf("create anubis app: %w", cerr)
			}
			if anubisApp, err = store.ApplicationByID(ctx, tenant.ID, id); err != nil {
				return err
			}
		}

		// Self catalog + admin role with the anubis:* pattern.
		keep := make([]string, 0, len(selfPermissions))
		for _, p := range selfPermissions {
			id, _, uerr := store.UpsertPermission(ctx, tenant.ID, anubisApp.ID, "anubis",
				repository.PermissionRecord{
					Resource: p.resource, Action: p.action,
					Description: p.description, Risk: p.risk, MinAssurance: 1,
				})
			if uerr != nil {
				return fmt.Errorf("permission %s:%s: %w", p.resource, p.action, uerr)
			}
			keep = append(keep, id)
		}
		if _, err := store.DeprecatePermissionsExcept(ctx, anubisApp.ID, keep); err != nil {
			return err
		}

		adminRole, err := store.RoleByName(ctx, tenant.ID, "anubis.admin")
		if err != nil {
			id, cerr := store.CreateRole(ctx, tenant.ID, repository.RoleRecord{
				Name: "anubis.admin", Description: "Full Anubis administration",
				AllowedRealmKinds: []string{"internal"},
			}, anubisApp.ID)
			if cerr != nil {
				return fmt.Errorf("create role: %w", cerr)
			}
			adminRole = &repository.RoleRecord{ID: id, Name: "anubis.admin"}
		}
		if err := store.SetRolePatterns(ctx, adminRole.ID, []string{"anubis:*"}); err != nil {
			return err
		}
		if err := store.RecomputeRole(ctx, adminRole.ID); err != nil {
			return err
		}

		// Admin identity + password + grant.
		identity, err := store.IdentityForLogin(ctx, tenant.ID, realm.ID, *adminUser)
		if err != nil || identity == nil {
			hash, herr := kdf.Hash(*adminPass)
			if herr != nil {
				return herr
			}
			id, cerr := store.CreateIdentity(ctx, repository.IdentityCreate{
				TenantID: tenant.ID, RealmID: realm.ID, Username: *adminUser,
				AssuranceLevel: 3, Status: "active",
			})
			if cerr != nil {
				return fmt.Errorf("create admin identity: %w", cerr)
			}
			if _, cerr := store.CreateCredential(ctx, repository.CredentialInput{
				IdentityID: id, TenantID: tenant.ID, Kind: "password", Secret: hash,
			}); cerr != nil {
				return cerr
			}
			identity = &domain.Identity{ID: id}
			logger.Info("admin identity created", "username", *adminUser)
		}

		grants, err := store.ListGrants(ctx, tenant.ID, identity.ID, false)
		if err != nil {
			return err
		}
		hasAdmin := false
		for _, g := range grants {
			if g.RoleID == adminRole.ID {
				hasAdmin = true
			}
		}
		if !hasAdmin {
			if _, err := store.CreateGrant(ctx, repository.GrantCreate{
				TenantID: tenant.ID, IdentityID: identity.ID, RoleID: adminRole.ID,
				GrantedBy: identity.ID, Reason: "bootstrap",
			}); err != nil {
				return fmt.Errorf("grant admin role: %w", err)
			}
		}
		logger.Info("bootstrap complete", "tenant", *tenantSlug, "admin", *adminUser)
		return nil
	})
}
