package authzapp

import (
	"context"
	"encoding/json"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// switchScopeInteractor implements SwitchScopeUsecase.
type switchScopeInteractor struct {
	sessions authport.SessionRepository
	nodes    scopeport.ScopeNodeRepository
	tenants  tenancyport.TenantRepository
	issuer   authapp.TokenIssuer
	tx       txm.TxManager
	audit    auditport.Auditor
}

func NewSwitchScopeInteractor(
	sessions authport.SessionRepository,
	nodes scopeport.ScopeNodeRepository,
	tenants tenancyport.TenantRepository,
	issuer authapp.TokenIssuer,
	tx txm.TxManager,
	audit auditport.Auditor,
) SwitchScopeUsecase {
	return &switchScopeInteractor{
		sessions: sessions, nodes: nodes, tenants: tenants,
		issuer: issuer, tx: tx, audit: audit,
	}
}

func (u *switchScopeInteractor) Execute(ctx context.Context, scopes map[string]string) (*authapp.TokenPair, error) {
	p, ok := authctx.From(ctx)
	if !ok || p.SessionID == "" {
		return nil, apperr.ErrUnauthenticated
	}
	if len(scopes) > 16 {
		return nil, apperr.ErrInvalidArgument.With("scopes", "too many axes")
	}
	// Every requested node must exist in this tenant on the axis it claims —
	// the schema's composite keys make lying about that unrepresentable, so
	// existence here IS validity.
	for axis, nodeID := range scopes {
		if !validate.ValidCode(axis) {
			return nil, apperr.ErrInvalidArgument.With("axis", axis)
		}
		node, err := u.nodes.ScopeNode(ctx, p.TenantID, nodeID)
		if err != nil || node == nil || node.Axis != axis || node.Status != "active" {
			return nil, apperr.ErrInvalidArgument.With("axis", axis).With("node", nodeID)
		}
	}
	raw, err := json.Marshal(scopes)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.Wrap(err)
	}

	var pair *authapp.TokenPair
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.sessions.UpdateSessionScopes(ctx, p.SessionID, raw); err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		view, err := u.sessions.SessionLive(ctx, p.SessionID)
		if err != nil {
			return apperr.ErrSessionRevoked
		}
		pair, err = u.issuer.Issue(ctx, authapp.IssueInput{
			Session:    view,
			TenantSlug: p.TenantSlug,
			AccessOnly: true,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, Action: "auth.scope_switch", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: raw,
	})
	return pair, nil
}
