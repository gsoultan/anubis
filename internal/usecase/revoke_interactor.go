package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// revokeInteractor implements RevokeUsecase.
type revokeInteractor struct {
	refresh  repository.RefreshRepository
	sessions repository.SessionRepository
	tenants  repository.TenantRepository
	audit    repository.Auditor
}

func NewRevokeInteractor(
	refresh repository.RefreshRepository,
	sessions repository.SessionRepository,
	tenants repository.TenantRepository,
	audit repository.Auditor,
) RevokeUsecase {
	return &revokeInteractor{refresh: refresh, sessions: sessions, tenants: tenants, audit: audit}
}

// Execute never reveals whether the presented token existed (RFC 7009 §2.2:
// invalid tokens answer success).
func (u *revokeInteractor) Execute(ctx context.Context, token, hint string) error {
	token = strings.TrimSpace(token)
	switch {
	case strings.HasPrefix(token, "anb_rt_"):
		info, err := u.refresh.RefreshByHash(ctx, secret.Hash(token))
		if err != nil || info == nil {
			return nil
		}
		_, _ = u.refresh.RevokeRefreshFamily(ctx, info.FamilyID)
		if _, err := u.sessions.RevokeSession(ctx, info.TenantID, info.SessionID, "token_revoked"); err == nil {
			_, _ = u.refresh.RevokeRefreshBySessions(ctx, []string{info.SessionID})
		}
		u.audit.Emit(ctx, repository.AuditEvent{
			TenantID: info.TenantID, ActorKind: "service",
			SessionID: info.SessionID, Action: "token.revoke", Result: "allow",
			IP: authctx.ClientIP(ctx), Detail: []byte(`{"type":"refresh"}`),
		})
	case strings.HasPrefix(token, "v4.public."):
		// Best effort: an access token revokes its session. Signature is NOT
		// required — a leaked token being revoked by whoever found it is the
		// desired outcome — but claims must parse.
		msg, _, _, err := paseto.Parse(token)
		if err != nil {
			return nil
		}
		var claims struct {
			Sid string `json:"sid"`
			Tid string `json:"tid"`
		}
		if json.Unmarshal(msg, &claims) != nil || claims.Sid == "" {
			return nil
		}
		tenant, err := u.tenants.TenantBySlug(ctx, claims.Tid)
		if err != nil || tenant == nil {
			return nil
		}
		if _, err := u.sessions.RevokeSession(ctx, tenant.ID, claims.Sid, "token_revoked"); err == nil {
			_, _ = u.refresh.RevokeRefreshBySessions(ctx, []string{claims.Sid})
		}
	}
	return nil
}
