package main

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apihttp "github.com/gsoultan/anubis/internal/api/http"
	auditpg "github.com/gsoultan/anubis/internal/audit/adapter/postgres"
	authhttp "github.com/gsoultan/anubis/internal/auth/adapter/http"
	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	authrpc "github.com/gsoultan/anubis/internal/auth/adapter/rpc"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	"github.com/gsoultan/anubis/internal/auth/app/clientcreds"
	"github.com/gsoultan/anubis/internal/auth/app/device"
	"github.com/gsoultan/anubis/internal/auth/app/enroll"
	"github.com/gsoultan/anubis/internal/auth/app/mfa"
	sessionapp "github.com/gsoultan/anubis/internal/auth/app/session"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
	authep "github.com/gsoultan/anubis/internal/auth/endpoint"
	authsvc "github.com/gsoultan/anubis/internal/auth/service"
	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	authzrpc "github.com/gsoultan/anubis/internal/authz/adapter/rpc"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzadmin "github.com/gsoultan/anubis/internal/authz/app/admin"
	authzep "github.com/gsoultan/anubis/internal/authz/endpoint"
	authzsvc "github.com/gsoultan/anubis/internal/authz/service"
	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	controlrpc "github.com/gsoultan/anubis/internal/control/adapter/rpc"
	controlapp "github.com/gsoultan/anubis/internal/control/app"
	controlsvc "github.com/gsoultan/anubis/internal/control/service"
	gatehttp "github.com/gsoultan/anubis/internal/gate/adapter/http"
	gatepg "github.com/gsoultan/anubis/internal/gate/adapter/postgres"
	gateapp "github.com/gsoultan/anubis/internal/gate/app"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	identityrpc "github.com/gsoultan/anubis/internal/identity/adapter/rpc"
	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
	identitysvc "github.com/gsoultan/anubis/internal/identity/service"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/platform/jobs"
	"github.com/gsoultan/anubis/internal/platform/mw"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	provisioningrpc "github.com/gsoultan/anubis/internal/provisioning/adapter/rpc"
	provisioningapp "github.com/gsoultan/anubis/internal/provisioning/app"
	provisioningsvc "github.com/gsoultan/anubis/internal/provisioning/service"
	scopepg "github.com/gsoultan/anubis/internal/scope/adapter/postgres"
	scoperpc "github.com/gsoultan/anubis/internal/scope/adapter/rpc"
	scopeapp "github.com/gsoultan/anubis/internal/scope/app"
	scopesvc "github.com/gsoultan/anubis/internal/scope/service"
	tenancypg "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres"
	tenancyrpc "github.com/gsoultan/anubis/internal/tenancy/adapter/rpc"
	tenancyapp "github.com/gsoultan/anubis/internal/tenancy/app"
	tenancysvc "github.com/gsoultan/anubis/internal/tenancy/service"

	"github.com/gsoultan/anubis/cmd/anubisd/syncengines"
	"github.com/gsoultan/anubis/internal/scope/adapter/feed"
)

// application is the composition root: it wires each bounded context's
// repositories, usecases, services and endpoints, then lets the contexts
// register their own transports. Nothing here contains business logic —
// this file exists so no other package has to know the whole system.
type application struct {
	clock     systemClock
	ring      *keyring.Manager
	auditor   *auditpg.ChainedAuditor
	issuer    authapp.TokenIssuer
	issuerURL string
	masterKey []byte
	// bootstrapTenantSlug is only used to render example page URLs for the
	// console; page lookup itself always resolves the tenant from the request.
	bootstrapTenantSlug string

	control  *controlpg.Repository
	identity *identitypg.Repository
	auth     *authpg.Repository
	authz    *authzpg.Repository
	scope    *scopepg.Repository
	tenancy  *tenancypg.Repository
	audit    *auditpg.Repository
	gate     *gatepg.Repository
}

func newApplication(ctx context.Context, cfg *config.Config, db *database.DB, logger *slog.Logger) (*application, error) {
	a := &application{
		clock:               systemClock{},
		masterKey:           cfg.MasterKey,
		issuerURL:           cfg.Issuer,
		bootstrapTenantSlug: cfg.DefaultTenant,
		control:             controlpg.New(db),
		identity:            identitypg.New(db),
		auth:                authpg.New(db),
		authz:               authzpg.New(db),
		scope:               scopepg.New(db),
		tenancy:             tenancypg.New(db),
		audit:               auditpg.New(db),
		gate:                gatepg.New(db),
	}
	ring, err := loadRing(ctx, logger, a.auth, cfg.MasterKey, cfg.AutoKeys)
	if err != nil {
		return nil, err
	}
	a.ring = ring
	a.auditor = auditpg.NewChainedAuditor(a.audit, logger)
	a.issuer = authapp.NewPasetoTokenIssuer(cfg.Issuer, ring, a.tenancy, a.identity,
		a.auth, a.auth, a.authz, a.tenancy, a.clock)
	return a, nil
}

func (a *application) close() { a.auditor.Close() }

// runMaintenance starts the recurring database maintenance every deployment
// needs. Replicas coordinate through advisory locks, so this is safe to run
// on every instance.
func (a *application) runMaintenance(ctx context.Context, db *database.DB, logger *slog.Logger) {
	retention := identityapp.NewRetentionInteractor(a.identity, a.identity, db, a.auditor)
	sched := jobs.NewScheduler(db, logger,
		maintenanceJobs(a.audit, a.auth, retention, a.auth, a.control, logger)...)
	go sched.Run(ctx)
}

// registerRPC mounts every context's Connect service on one mux.
func (a *application) registerRPC(rpc *http.ServeMux, opts connect.HandlerOption,
	limiter *ratelimit.Limiter, logger *slog.Logger) {

	// --- auth context -------------------------------------------------------
	backchannel := sessionapp.NewBackchannelLogout(a.issuerURL, a.ring, a.tenancy,
		authpg.NewHTTPBackchannelNotifier(logger), a.clock)
	login := signin.NewLoginInteractor(a.tenancy, a.identity, a.identity, a.identity,
		a.auth, a.auth, a.issuer, a.ring, a.auth, a.clock, a.auditor)
	verifyMfa := mfa.NewVerifyMfaInteractor(a.ring, a.auth, a.identity, a.identity,
		a.identity, a.tenancy, a.auth, a.issuer, a.auth, a.clock, a.auditor)
	refresh := tokenapp.NewRefreshInteractor(a.auth, a.auth, a.tenancy, a.issuer, a.auth, a.auditor)
	logout := sessionapp.NewLogoutInteractor(a.auth, a.auth, a.identity, a.auth, a.auditor, backchannel)
	dev := device.NewDeviceInteractor(a.tenancy, a.identity, a.identity, a.identity,
		a.auth, a.auth, a.issuer, a.auth, a.clock, a.auditor)
	reg := registration.NewRegisterInteractor(a.tenancy, a.identity, a.identity, a.identity,
		a.identity, a.auth, a.auth, a.clock, a.auditor)
	verifyEmail := registration.NewVerifyEmailInteractor(a.auth, a.identity)
	introspect := tokenapp.NewIntrospectInteractor(a.issuerURL, a.ring, a.auth, a.tenancy, a.clock)
	revoke := tokenapp.NewRevokeInteractor(a.auth, a.auth, a.tenancy, a.auditor)
	enrollment := enroll.NewEnrollmentInteractor(a.issuerURL, a.ring, a.identity,
		a.identity, a.auth, a.auth, a.clock, a.auditor)
	clientCreds := clientcreds.NewClientCredentialsInteractor(a.issuerURL, a.ring,
		a.tenancy, a.tenancy, a.clock, a.auditor)
	getMe := sessionapp.NewGetMeInteractor(a.identity, a.authz)
	listSessions := sessionapp.NewListSessionsInteractor(a.auth)

	authService := authsvc.NewAuthService(login, verifyMfa, refresh,
		logout, logout.All(), logout.Session(), dev, dev.Verify(), reg, verifyEmail,
		enrollment, clientCreds)
	tokenService := authsvc.NewTokenService(introspect, revoke)
	sessionService := authsvc.NewSessionService(getMe, listSessions, logout.Session())

	rpc.Handle(anubisv1connect.NewAuthServiceHandler(
		authrpc.NewAuthHandler(authep.NewAuthEndpoints(authService, logger, limiter)), opts))
	rpc.Handle(anubisv1connect.NewTokenServiceHandler(
		authrpc.NewTokenHandler(authep.NewTokenEndpoints(tokenService, logger)), opts))
	rpc.Handle(anubisv1connect.NewSessionServiceHandler(
		authrpc.NewSessionHandler(authep.NewSessionEndpoints(sessionService, logger)), opts))

	// --- authz context ------------------------------------------------------
	authorize := authzapp.NewAuthorizeInteractor(a.authz, a.clock, a.auditor)
	explain := authzapp.NewExplainInteractor(a.authz)
	switchScope := authzapp.NewSwitchScopeInteractor(a.auth, a.scope, a.tenancy, a.issuer, a.auth, a.auditor)
	authzService := authzsvc.NewAuthzService(authorize, explain, switchScope)
	rpc.Handle(anubisv1connect.NewAuthzServiceHandler(
		authzrpc.NewAuthzHandler(authzep.NewAuthzEndpoints(authzService, logger)), opts))

	// --- admin planes -------------------------------------------------------
	f := mw.NewFactory(logger)

	identityAdmin := identityapp.NewIdentityAdminInteractor(a.control, a.clock.Now,
		a.identity, a.identity,
		a.identity, a.identity, a.identity, a.identity, a.auth, a.auth, a.tenancy,
		a.auth, a.clock, a.auditor)
	// The master key goes to exactly one interactor: the one that seals
	// identity attributes (ADR-0013). Nothing else in the identity plane
	// needs it, so nothing else is given it.
	identityAttrs := identityapp.NewAttributesInteractor(
		a.control, a.clock.Now, a.identity, a.identity, a.auditor, a.masterKey)
	rpc.Handle(anubisv1connect.NewIdentityAdminServiceHandler(
		identityrpc.NewIdentityAdminHandler(
			identitysvc.NewIdentityAdminService(identityAdmin, identityAttrs), f), opts))

	syncengines.Register()
	scopeAdmin := scopeapp.NewScopeAdminInteractor(a.authz, a.control, a.clock.Now,
		a.scope, a.scope, a.scope,
		feed.NewFetcher(), a.auth, a.auditor)
	rpc.Handle(anubisv1connect.NewScopeAdminServiceHandler(
		scoperpc.NewScopeAdminHandler(scopesvc.NewScopeAdminService(scopeAdmin), f), opts))

	authzAdmin := authzadmin.NewAuthzAdminInteractor(a.control, a.clock.Now,
		a.authz, a.authz, a.authz,
		a.authz, a.tenancy, a.tenancy, a.auth, a.auditor)
	rpc.Handle(anubisv1connect.NewAuthzAdminServiceHandler(
		authzrpc.NewAuthzAdminHandler(authzsvc.NewAuthzAdminService(authzAdmin), f), opts))

	// --- control plane (ADR-0011) -------------------------------------------
	// Who operates this installation, and over which tenants. The owner that
	// setup creates is an ordinary identity in the platform tenant, so it
	// appears in this list like any other operator.
	operatorAdmin := controlapp.NewOperatorAdminInteractor(a.control, a.tenancy,
		a.control, a.control, a.clock, a.identity, a.auditor)
	// Machine credentials for operators: how a pipeline administers the
	// installation now that tenant identities cannot (0029).
	platformKeys := controlapp.NewPlatformAPIKeyInteractor(a.control, a.control,
		a.control, a.clock, a.auditor)
	rpc.Handle(anubisv1connect.NewPlatformAdminServiceHandler(
		controlrpc.NewPlatformAdminHandler(
			controlsvc.NewControlService(operatorAdmin, platformKeys), f, a.clock.Now), opts))

	// The operators' own door. Separate from AuthService, which resolves a
	// tenant identity — a platform user is deliberately not one.
	platformAuth := controlapp.NewPlatformAuthInteractor(a.control, a.control, a.tenancy,
		a.control, a.ring, a.clock, a.auditor, a.issuerURL, a.masterKey)
	rpc.Handle(anubisv1connect.NewPlatformAuthServiceHandler(
		controlrpc.NewPlatformAuthHandler(platformAuth, f, limiter), opts))

	// --- provisioning context -----------------------------------------------
	// Bulk import is orchestration, not a second way in: every write goes
	// through the identity and authz admin usecases wired just above, so it
	// inherits their permission checks, their validation and their audit
	// events instead of carrying a copy of any of them.
	importer := provisioningapp.NewImportInteractor(a.control, a.clock.Now,
		a.identity, identityAdmin,
		a.authz, authzAdmin, a.scope, a.clock, a.identity, a.auditor)
	rpc.Handle(anubisv1connect.NewProvisioningServiceHandler(
		provisioningrpc.NewProvisioningHandler(
			provisioningsvc.NewProvisioningService(importer), f), opts))

	tenantAdmin := tenancyapp.NewTenantAdminInteractor(a.control, a.clock.Now,
		a.tenancy, a.auth, a.identity,
		a.tenancy, a.tenancy, a.audit, a.auth, a.tenancy, a.auditor,
		&storeKeyRotator{keys: a.auth, master: a.masterKey}, a.auditor)
	pageAdmin := tenancyapp.NewPageAdminInteractor(a.control, a.clock.Now,
		a.tenancy, a.tenancy, a.auditor)
	// The overview reads a sliver of several contexts through narrow ports,
	// each satisfied structurally by that context's repository.
	dashboard := tenancyapp.NewDashboardInteractor(a.control, a.clock.Now,
		a.identity, a.authz, a.scope, a.audit, a.ring)
	rpc.Handle(anubisv1connect.NewTenantAdminServiceHandler(
		tenancyrpc.NewTenantAdminHandler(
			tenancysvc.NewTenantAdminService(tenantAdmin, pageAdmin, dashboard), f,
			a.issuerURL, a.bootstrapTenantSlug), opts))
}

// registerHTTP lets each context mount its protocol-shaped routes (OIDC,
// key discovery, forward auth) on the shared server.
func (a *application) registerHTTP(ctx context.Context, srv *apihttp.Server,
	cfg *config.Config, health *apihttp.HealthHandler,
	limiter *ratelimit.Limiter, logger *slog.Logger) {

	// Read before sign-in, so the console can fill in the tenant instead of
	// asking someone to recall a slug.
	console := apihttp.NewConsoleHandler(a.control, cfg.Issuer, logger)
	srv.HandleFunc("GET /v1/console-config", console.Config)

	wellKnown := authhttp.NewWellKnownHandler(cfg.Issuer, a.ring)
	srv.HandleFunc("GET /.well-known/anubis-keys.json", wellKnown.Keys)
	srv.HandleFunc("GET /.well-known/openid-configuration", wellKnown.OpenIDConfiguration)

	oidc := authhttp.NewOIDCHandler(cfg.Issuer, a.tenancy, a.identity, a.identity,
		a.identity, a.identity, a.auth, a.auth, a.tenancy, a.tenancy, a.auth,
		cfg.DefaultTenant, cfg.Env == "prod", a.issuer, a.clock, a.auditor, limiter, logger)
	srv.HandleFunc("GET /v1/authorize", oidc.Authorize)
	srv.HandleFunc("POST /v1/login", oidc.LoginForm)
	srv.HandleFunc("POST /v1/token", oidc.Token)
	// RP-initiated logout, OIDC-shaped: GET asks, POST performs.
	srv.HandleFunc("GET /v1/logout", oidc.LogoutPage)
	srv.HandleFunc("POST /v1/logout", oidc.LogoutSubmit)
	// Each page has its own URL, which is the point of having many of them.
	srv.HandleFunc("GET /p/{tenant}/{kind}/{slug}", oidc.ServePage)

	snaps := gateapp.NewManager(a.gate, a.gate, cfg.SnapshotMaxAge, logger)
	go snaps.Run(ctx)
	// Readiness must fail while the snapshot is too stale to serve from:
	// past that age the gate fails closed, so this instance is denying
	// traffic and should leave the load balancer.
	health.WithSnapshot(snaps)
	gate := gatehttp.NewGateHandler(cfg.Issuer, a.ring, snaps)
	srv.HandleFunc("POST /v1/gate/check", gate.Check)
	srv.HandleFunc("GET /v1/gate/check", gate.Check)
}
