package tenancyapp

import (
	"context"
	"encoding/json"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/authz/guard"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// ChainVerifier abstracts audit chain verification (implemented by the
// postgres ChainedAuditor).
type ChainVerifier interface {
	VerifyChain(ctx context.Context, tenantID string, from, to *time.Time) (checked int64, brokenAt int64, err error)
}

// KeyRotator abstracts the prepare-a-pending-key step so the interactor
// stays free of crypto wiring (cmd owns master-key material).
type KeyRotator interface {
	PrepareKey(ctx context.Context, purpose string) (*authdomain.KeyRecord, error)
}

// tenantAdminInteractor implements TenantAdminUsecase.
type tenantAdminInteractor struct {
	guard    *guard.Guard
	tenants  tenancyport.TenantRepository
	realms   identityport.RealmAdminRepository
	apps     tenancyport.ApplicationRepository
	routes   tenancyport.RouteRepository
	auditRd  auditport.AuditReadRepository
	keys     authport.KeyRepository
	signin   tenancyport.SigninPageRepository
	verifier ChainVerifier
	rotator  KeyRotator
	audit    auditport.Auditor
}

func NewTenantAdminInteractor(
	authz authzport.AuthzRepository,
	tenants tenancyport.TenantRepository,
	realms identityport.RealmAdminRepository,
	apps tenancyport.ApplicationRepository,
	routes tenancyport.RouteRepository,
	auditRd auditport.AuditReadRepository,
	keys authport.KeyRepository,
	signin tenancyport.SigninPageRepository,
	verifier ChainVerifier,
	rotator KeyRotator,
	audit auditport.Auditor,
) TenantAdminUsecase {
	return &tenantAdminInteractor{
		guard: guard.New(authz), tenants: tenants, realms: realms,
		apps: apps, routes: routes, auditRd: auditRd, keys: keys,
		signin: signin, verifier: verifier, rotator: rotator, audit: audit,
	}
}

func (u *tenantAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, target string, detail map[string]string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	})
}

func (u *tenantAdminInteractor) ListTenants(ctx context.Context) ([]tenancydomain.TenantRef, error) {
	if _, err := u.guard.Require(ctx, "anubis:tenant:admin"); err != nil {
		return nil, err
	}
	return u.tenants.ListTenants(ctx)
}

func (u *tenantAdminInteractor) CreateTenant(ctx context.Context, slug, name string) (*tenancydomain.TenantRef, error) {
	p, err := u.guard.Require(ctx, "anubis:tenant:admin")
	if err != nil {
		return nil, err
	}
	if !validate.ValidSlug(slug) {
		return nil, apperr.ErrInvalidArgument.With("slug", slug)
	}
	t, err := u.tenants.CreateTenant(ctx, slug, name)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "tenant.create", t.ID, map[string]string{"slug": slug})
	return t, nil
}

func (u *tenantAdminInteractor) ListRealms(ctx context.Context) ([]identitydomain.RealmRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.realms.ListRealms(ctx, p.TenantID)
}

func (u *tenantAdminInteractor) CreateRealm(ctx context.Context, r identitydomain.RealmRecord) (*identitydomain.RealmRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:realm:admin")
	if err != nil {
		return nil, err
	}
	if !validate.ValidCode(r.Code) {
		return nil, apperr.ErrInvalidArgument.With("code", r.Code)
	}
	id, err := u.realms.CreateRealm(ctx, p.TenantID, r)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "realm.create", id, map[string]string{"code": r.Code, "kind": r.Kind})
	return u.realmByID(ctx, p.TenantID, id)
}

func (u *tenantAdminInteractor) UpdateRealm(ctx context.Context, r identitydomain.RealmRecord) (*identitydomain.RealmRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:realm:admin")
	if err != nil {
		return nil, err
	}
	if err := u.realms.UpdateRealm(ctx, p.TenantID, r); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "realm.update", r.ID, map[string]string{"code": r.Code})
	return u.realmByID(ctx, p.TenantID, r.ID)
}

func (u *tenantAdminInteractor) realmByID(ctx context.Context, tenantID, id string) (*identitydomain.RealmRecord, error) {
	list, err := u.realms.ListRealms(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, apperr.ErrNotFound
}

func (u *tenantAdminInteractor) ListRealmCategories(ctx context.Context, realmID string) ([]identitydomain.RealmCategoryRecord, error) {
	if _, err := u.guard.Require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.realms.ListRealmCategories(ctx, realmID)
}

func (u *tenantAdminInteractor) CreateRealmCategory(ctx context.Context, c identitydomain.RealmCategoryRecord) (*identitydomain.RealmCategoryRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:realm:admin")
	if err != nil {
		return nil, err
	}
	id, err := u.realms.CreateRealmCategory(ctx, p.TenantID, c)
	if err != nil {
		return nil, err
	}
	c.ID = id
	u.emit(ctx, p, "realm.category_create", id, map[string]string{"code": c.Code})
	return &c, nil
}

func (u *tenantAdminInteractor) ListApplications(ctx context.Context) ([]tenancydomain.ApplicationRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.apps.ListApplications(ctx, p.TenantID)
}

// CreateApplication returns the client secret exactly once for confidential
// kinds; only its hash persists.
func (u *tenantAdminInteractor) CreateApplication(ctx context.Context, a tenancydomain.ApplicationRecord) (*tenancydomain.ApplicationRecord, string, error) {
	p, err := u.guard.Require(ctx, "anubis:application:admin")
	if err != nil {
		return nil, "", err
	}
	if !validate.ValidSlug(a.Slug) {
		return nil, "", apperr.ErrInvalidArgument.With("slug", a.Slug)
	}
	clientSecret := ""
	if a.Kind == "server" || a.Kind == "service" {
		s, serr := secret.New(32)
		if serr != nil {
			return nil, "", apperr.ErrInternal.Wrap(serr)
		}
		clientSecret = s
		a.ClientSecretHash = secret.Hex(secret.Hash(s))
	}
	id, err := u.apps.CreateApplication(ctx, p.TenantID, a)
	if err != nil {
		return nil, "", err
	}
	u.emit(ctx, p, "application.create", id, map[string]string{"slug": a.Slug, "kind": a.Kind})
	rec, err := u.apps.ApplicationByID(ctx, p.TenantID, id)
	return rec, clientSecret, err
}

func (u *tenantAdminInteractor) UpdateApplication(ctx context.Context, a tenancydomain.ApplicationRecord) (*tenancydomain.ApplicationRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:application:admin")
	if err != nil {
		return nil, err
	}
	if err := u.apps.UpdateApplication(ctx, p.TenantID, a); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "application.update", a.ID, map[string]string{"slug": a.Slug})
	return u.apps.ApplicationByID(ctx, p.TenantID, a.ID)
}

func (u *tenantAdminInteractor) RotateClientSecret(ctx context.Context, applicationID string) (string, error) {
	p, err := u.guard.Require(ctx, "anubis:application:admin")
	if err != nil {
		return "", err
	}
	s, err := secret.New(32)
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	if err := u.apps.SetClientSecretHash(ctx, p.TenantID, applicationID, secret.Hex(secret.Hash(s))); err != nil {
		return "", err
	}
	u.emit(ctx, p, "application.secret_rotate", applicationID, nil)
	return s, nil
}

func (u *tenantAdminInteractor) ListRoutePolicies(ctx context.Context, applicationSlug string) ([]tenancydomain.RoutePolicyRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	app, err := u.apps.ApplicationBySlug(ctx, p.TenantID, applicationSlug)
	if err != nil {
		return nil, apperr.ErrNotFound.With("application", applicationSlug)
	}
	list, err := u.routes.ListRoutePolicies(ctx, app.ID)
	for i := range list {
		list[i].AppSlug = app.Slug
	}
	return list, err
}

func (u *tenantAdminInteractor) QueryAudit(ctx context.Context, q auditdomain.AuditQuery) ([]auditdomain.AuditRecord, string, error) {
	p, err := u.guard.Require(ctx, "anubis:audit:read")
	if err != nil {
		return nil, "", err
	}
	list, err := u.auditRd.QueryAudit(ctx, p.TenantID, q)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if q.Limit > 0 && len(list) == q.Limit {
		next = encodeSeq(list[len(list)-1].Seq)
	}
	return list, next, nil
}

func (u *tenantAdminInteractor) VerifyAuditChain(ctx context.Context, from, to *time.Time) (int64, int64, error) {
	p, err := u.guard.Require(ctx, "anubis:audit:read")
	if err != nil {
		return 0, 0, err
	}
	return u.verifier.VerifyChain(ctx, p.TenantID, from, to)
}

func (u *tenantAdminInteractor) ListSigningKeys(ctx context.Context) ([]authdomain.KeyRecord, error) {
	if _, err := u.guard.Require(ctx, "anubis:key:admin"); err != nil {
		return nil, err
	}
	return u.keys.SigningKeys(ctx)
}

// RotateSigningKey creates a PENDING key (publish before use); promotion is a
// separate deliberate step (anubisd keys promote / a later RPC).
func (u *tenantAdminInteractor) RotateSigningKey(ctx context.Context, purpose string) (*authdomain.KeyRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:key:admin")
	if err != nil {
		return nil, err
	}
	rec, err := u.rotator.PrepareKey(ctx, purpose)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "key.prepare", rec.Kid, map[string]string{"purpose": purpose})
	return rec, nil
}

func (u *tenantAdminInteractor) GetCatalogVersion(ctx context.Context) (int64, time.Time, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return 0, time.Time{}, err
	}
	return u.tenants.CatalogVersion(ctx, p.TenantID)
}

func (u *tenantAdminInteractor) GetSigninPage(ctx context.Context) ([]byte, time.Time, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, time.Time{}, err
	}
	cfg, at, err := u.signin.SigninPage(ctx, p.TenantID)
	if err != nil {
		return []byte("{}"), time.Time{}, nil // absent config is an empty page
	}
	return cfg, at, nil
}

func (u *tenantAdminInteractor) PutSigninPage(ctx context.Context, config []byte) error {
	p, err := u.guard.Require(ctx, "anubis:signin:admin")
	if err != nil {
		return err
	}
	if !json.Valid(config) {
		return apperr.ErrInvalidArgument.With("config", "invalid JSON")
	}
	if err := u.signin.PutSigninPage(ctx, p.TenantID, config); err != nil {
		return err
	}
	u.emit(ctx, p, "signin.update", p.TenantID, nil)
	return nil
}

func encodeSeq(n int64) string {
	digits := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
