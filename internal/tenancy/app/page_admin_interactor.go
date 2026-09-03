package tenancyapp

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/authz/guard"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// pageAdminInteractor implements PageAdminUsecase.
type pageAdminInteractor struct {
	guard *guard.Guard
	pages tenancyport.AuthPageRepository
	apps  tenancyport.ApplicationRepository
	audit auditport.Auditor
}

func NewPageAdminInteractor(
	// ops lets a PLATFORM OPERATOR administer this tenant's sign-in pages
	// (ADR-0011). Their authority is an assignment, not a grant, so the
	// guard has to ask the control plane rather than authorize().
	ops guard.OperatorAuthority,
	clockNow func() time.Time,
	pages tenancyport.AuthPageRepository,
	apps tenancyport.ApplicationRepository,
	audit auditport.Auditor,
) PageAdminUsecase {
	return &pageAdminInteractor{guard: guard.New().WithOperators(ops, clockNow), pages: pages, apps: apps, audit: audit}
}

func (u *pageAdminInteractor) ListAuthPages(ctx context.Context, kind string) ([]tenancydomain.AuthPage, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	if kind != "" && !validKind(kind) {
		return nil, apperr.ErrInvalidArgument.With("kind", kind)
	}
	return u.pages.ListAuthPages(ctx, p.TenantID, kind)
}

func (u *pageAdminInteractor) GetAuthPage(ctx context.Context, id string) (*tenancydomain.AuthPage, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.pages.AuthPage(ctx, p.TenantID, id)
}

func (u *pageAdminInteractor) CreateAuthPage(ctx context.Context, in tenancydomain.AuthPageInput) (*tenancydomain.AuthPage, error) {
	p, err := u.guard.Require(ctx, "anubis:signin:admin")
	if err != nil {
		return nil, err
	}
	config, err := u.normalise(ctx, p.TenantID, &in)
	if err != nil {
		return nil, err
	}
	in.Config = config
	id, err := u.pages.CreateAuthPage(ctx, p.TenantID, in)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "page.create", id, in.Kind, in.Slug)
	return u.pages.AuthPage(ctx, p.TenantID, id)
}

func (u *pageAdminInteractor) UpdateAuthPage(ctx context.Context, in tenancydomain.AuthPageInput) (*tenancydomain.AuthPage, error) {
	p, err := u.guard.Require(ctx, "anubis:signin:admin")
	if err != nil {
		return nil, err
	}
	existing, err := u.pages.AuthPage(ctx, p.TenantID, in.ID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	// Kind and slug are the page's identity: its URL is published, and
	// changing either silently breaks every link that points at it.
	in.Kind = existing.Kind
	in.Slug = existing.Slug
	config, err := u.normalise(ctx, p.TenantID, &in)
	if err != nil {
		return nil, err
	}
	in.Config = config
	// Disabling the default would leave /v1/authorize with nothing to render.
	if existing.IsDefault && in.Status == "disabled" {
		return nil, apperr.ErrInvalidArgument.
			With("status", "promote another page to default before disabling this one")
	}
	if err := u.pages.UpdateAuthPage(ctx, p.TenantID, in); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "page.update", in.ID, existing.Kind, existing.Slug)
	return u.pages.AuthPage(ctx, p.TenantID, in.ID)
}

func (u *pageAdminInteractor) DeleteAuthPage(ctx context.Context, id string) error {
	p, err := u.guard.Require(ctx, "anubis:signin:admin")
	if err != nil {
		return err
	}
	page, err := u.pages.AuthPage(ctx, p.TenantID, id)
	if err != nil {
		return apperr.ErrNotFound
	}
	if page.IsDefault {
		return apperr.ErrInvalidArgument.
			With("page", "the default page cannot be deleted; promote another first")
	}
	if err := u.pages.DeleteAuthPage(ctx, p.TenantID, id); err != nil {
		return err
	}
	u.emit(ctx, p, "page.delete", id, page.Kind, page.Slug)
	return nil
}

func (u *pageAdminInteractor) SetDefaultAuthPage(ctx context.Context, id string) error {
	p, err := u.guard.Require(ctx, "anubis:signin:admin")
	if err != nil {
		return err
	}
	page, err := u.pages.AuthPage(ctx, p.TenantID, id)
	if err != nil {
		return apperr.ErrNotFound
	}
	if err := u.pages.SetDefaultAuthPage(ctx, p.TenantID, page.Kind, id); err != nil {
		return err
	}
	u.emit(ctx, p, "page.set_default", id, page.Kind, page.Slug)
	return nil
}

func (u *pageAdminInteractor) PreviewAuthPage(ctx context.Context, kind string, config []byte) error {
	if _, err := u.guard.Require(ctx, "anubis:identity:read"); err != nil {
		return err
	}
	if !validKind(kind) {
		return apperr.ErrInvalidArgument.With("kind", kind)
	}
	_, err := pagecfg.Parse(pagecfg.Kind(kind), config)
	return err
}

// normalise validates the slug, the kind, the application binding and the
// config, returning the canonical config to store. Storing the parsed form
// means the render path never has to cope with a half-valid page.
func (u *pageAdminInteractor) normalise(ctx context.Context, tenantID string, in *tenancydomain.AuthPageInput) ([]byte, error) {
	if !validKind(in.Kind) {
		return nil, apperr.ErrInvalidArgument.With("kind", in.Kind)
	}
	if !validate.ValidSlug(in.Slug) {
		return nil, apperr.ErrInvalidArgument.With("slug", "lowercase letters, digits, - and _")
	}
	if in.Name == "" {
		return nil, apperr.ErrInvalidArgument.With("name", "required")
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "disabled" {
		return nil, apperr.ErrInvalidArgument.With("status", in.Status)
	}
	// A page answers for an application OR a population, never both:
	// resolution would have to pick one, and whichever it picked would
	// surprise somebody. auth_pages_one_binding (migration 0041) refuses the
	// row, but a CHECK violation names a constraint rather than a field, and
	// the console needs to point at the input the operator got wrong.
	if in.ApplicationID != "" && in.RealmID != "" {
		return nil, apperr.ErrInvalidArgument.
			With("realm_id", "a page answers for an application or a population, not both")
	}
	if in.ApplicationID != "" {
		if _, err := u.apps.ApplicationByID(ctx, tenantID, in.ApplicationID); err != nil {
			return nil, apperr.ErrInvalidArgument.With("application_id", "unknown application")
		}
	}
	cfg, err := pagecfg.Parse(pagecfg.Kind(in.Kind), in.Config)
	if err != nil {
		return nil, err
	}
	return cfg.Marshal()
}

func (u *pageAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, id, kind, slug string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: id, Action: action, Result: "allow",
		IP:     authctx.ClientIP(ctx),
		Detail: jsonx.Must(map[string]string{"kind": kind, "slug": slug}),
	})
}

func validKind(k string) bool { return k == "signin" || k == "signout" }
