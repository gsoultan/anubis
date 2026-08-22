package usecase

import (
	"context"
	"encoding/json"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// switchScopeInteractor implements SwitchScopeUsecase.
type switchScopeInteractor struct {
	sessions repository.SessionRepository
	nodes    repository.ScopeNodeRepository
	tenants  repository.TenantRepository
	issuer   TokenIssuer
	tx       repository.TxManager
	audit    repository.Auditor
}

func NewSwitchScopeInteractor(
	sessions repository.SessionRepository,
	nodes repository.ScopeNodeRepository,
	tenants repository.TenantRepository,
	issuer TokenIssuer,
	tx repository.TxManager,
	audit repository.Auditor,
) SwitchScopeUsecase {
	return &switchScopeInteractor{
		sessions: sessions, nodes: nodes, tenants: tenants,
		issuer: issuer, tx: tx, audit: audit,
	}
}

func (u *switchScopeInteractor) Execute(ctx context.Context, scopes map[string]string) (*TokenPair, error) {
	p, ok := authctx.From(ctx)
	if !ok || p.SessionID == "" {
		return nil, domain.ErrUnauthenticated
	}
	if len(scopes) > 16 {
		return nil, domain.ErrInvalidArgument.With("scopes", "too many axes")
	}
	// Every requested node must exist in this tenant on the axis it claims —
	// the schema's composite keys make lying about that unrepresentable, so
	// existence here IS validity.
	for axis, nodeID := range scopes {
		if !domain.ValidCode(axis) {
			return nil, domain.ErrInvalidArgument.With("axis", axis)
		}
		node, err := u.nodes.ScopeNode(ctx, p.TenantID, nodeID)
		if err != nil || node == nil || node.Axis != axis || node.Status != "active" {
			return nil, domain.ErrInvalidArgument.With("axis", axis).With("node", nodeID)
		}
	}
	raw, err := json.Marshal(scopes)
	if err != nil {
		return nil, domain.ErrInvalidArgument.Wrap(err)
	}

	var pair *TokenPair
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.sessions.UpdateSessionScopes(ctx, p.SessionID, raw); err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		view, err := u.sessions.SessionLive(ctx, p.SessionID)
		if err != nil {
			return domain.ErrSessionRevoked
		}
		pair, err = u.issuer.Issue(ctx, IssueInput{
			Session:    view,
			TenantSlug: p.TenantSlug,
			AccessOnly: true,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, Action: "auth.scope_switch", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: raw,
	})
	return pair, nil
}
