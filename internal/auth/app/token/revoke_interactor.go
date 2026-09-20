package tokenapp

import (
	"context"
	"encoding/json"
	"strings"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/accesstoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// revokeInteractor implements RevokeUsecase.
type revokeInteractor struct {
	refresh  authport.RefreshRepository
	sessions authport.SessionRepository
	tenants  tenancyport.TenantRepository
	audit    auditport.Auditor
}

func NewRevokeInteractor(
	refresh authport.RefreshRepository,
	sessions authport.SessionRepository,
	tenants tenancyport.TenantRepository,
	audit auditport.Auditor,
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
		// These errors were discarded, so a revocation that failed returned
		// success to the caller AND recorded result=allow. RFC 7009 wants an
		// unknown token to look like a successful revocation — that is the
		// `return nil` above — but it does not ask us to claim we revoked a
		// token we did not. A caller told "revoked" does not retry.
		if _, rerr := u.refresh.RevokeRefreshFamily(ctx, info.FamilyID); rerr != nil {
			u.auditRevokeFailed(ctx, info, "family")
			return apperr.ErrInternal.Wrap(rerr)
		}
		if _, rerr := u.sessions.RevokeSession(ctx, info.TenantID, info.SessionID, "token_revoked"); rerr != nil {
			u.auditRevokeFailed(ctx, info, "session")
			return apperr.ErrInternal.Wrap(rerr)
		}
		if _, rerr := u.refresh.RevokeRefreshBySessions(ctx, []string{info.SessionID}); rerr != nil {
			u.auditRevokeFailed(ctx, info, "session_tokens")
			return apperr.ErrInternal.Wrap(rerr)
		}
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID: info.TenantID, ActorKind: "service",
			SessionID: info.SessionID, Action: "token.revoke", Result: "allow",
			IP: authctx.ClientIP(ctx), Detail: []byte(`{"type":"refresh"}`),
		})
	case accesstoken.IsAccessToken(token):
		// Best effort: an access token revokes its session. Signature is NOT
		// required — a leaked token being revoked by whoever found it is the
		// desired outcome — but claims must parse.
		//
		// Both formats, not just PASETO: this branch was gated on the
		// "v4.public." prefix, so an application issuing JWS would have had
		// its revocations silently do nothing.
		msg, err := accesstoken.ClaimsUnverified(token)
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

// auditRevokeFailed records a revocation that was asked for and did not
// happen. Separate from the allow event so a search for "was this token
// revoked" cannot match an attempt.
func (u *revokeInteractor) auditRevokeFailed(ctx context.Context, info *authdomain.RefreshInfo, stage string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: info.TenantID, ActorKind: "service",
		SessionID: info.SessionID, Action: "token.revoke", Result: "error",
		IP:     authctx.ClientIP(ctx),
		Detail: jsonx.Must(map[string]string{"type": "refresh", "failed_at": stage}),
	})
}
