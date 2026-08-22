package tokenapp

import (
	"context"
	"errors"
	"strings"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// refreshInteractor implements RefreshUsecase.
type refreshInteractor struct {
	refresh  authport.RefreshRepository
	sessions authport.SessionRepository
	tenants  tenancyport.TenantRepository
	issuer   authapp.TokenIssuer
	tx       txm.TxManager
	audit    auditport.Auditor
}

func NewRefreshInteractor(
	refresh authport.RefreshRepository,
	sessions authport.SessionRepository,
	tenants tenancyport.TenantRepository,
	issuer authapp.TokenIssuer,
	tx txm.TxManager,
	audit auditport.Auditor,
) RefreshUsecase {
	return &refreshInteractor{
		refresh: refresh, sessions: sessions, tenants: tenants,
		issuer: issuer, tx: tx, audit: audit,
	}
}

// Sentinels that abort the rotation transaction. The security RESPONSE to
// them (family revocation, session revocation) must happen OUTSIDE that
// transaction: an aborted transaction rolls its writes back, and a rolled-
// back revocation is a successor token that still works — the exact hole
// theft detection exists to close.
var (
	errClaimFailed = errors.New("refresh claim failed")
	errSessionDead = errors.New("session dead")
)

func (u *refreshInteractor) Execute(ctx context.Context, in RefreshInput) (*authapp.TokenPair, error) {
	token := strings.TrimSpace(in.RefreshToken)
	if !strings.HasPrefix(token, "anb_rt_") || len(token) > 256 {
		return nil, apperr.ErrRefreshInvalid
	}
	hash := secret.Hash(token)

	var pair *authapp.TokenPair
	var deadClaim *authdomain.RefreshClaim
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		claim, err := u.refresh.ClaimRefresh(ctx, hash)
		if err != nil {
			return errClaimFailed
		}
		view, err := u.sessions.SessionLive(ctx, claim.SessionID)
		if err != nil {
			deadClaim = claim
			return errSessionDead
		}
		tenant, err := u.tenants.TenantByID(ctx, claim.TenantID)
		if err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		pair, err = u.issuer.Issue(ctx, authapp.IssueInput{
			Session:    view,
			TenantSlug: tenant.Slug,
			RotateFrom: claim,
		})
		if err != nil {
			return err
		}
		u.sessions.TouchSession(ctx, view.ID)
		return nil
	})
	switch {
	case err == nil:
		return pair, nil
	case errors.Is(err, errClaimFailed):
		// Outside the aborted transaction: these writes must survive.
		return nil, u.respondToFailedClaim(ctx, hash)
	case errors.Is(err, errSessionDead):
		// The claim consumed the token in a rolled-back tx; kill the family
		// for real so nothing in it can be replayed later.
		_, _ = u.refresh.RevokeRefreshFamily(ctx, deadClaim.FamilyID)
		return nil, apperr.ErrSessionRevoked
	default:
		return nil, err
	}
}

// respondToFailedClaim distinguishes "never existed / expired" from the
// security-critical case: the token EXISTS but was already consumed or
// revoked — token theft, detected. Runs in its own committed writes.
func (u *refreshInteractor) respondToFailedClaim(ctx context.Context, hash []byte) error {
	info, err := u.refresh.RefreshByHash(ctx, hash)
	if err != nil || info == nil {
		return apperr.ErrRefreshInvalid
	}
	switch info.Status {
	case "consumed", "revoked":
		_, _ = u.refresh.RevokeRefreshFamily(ctx, info.FamilyID)
		if _, rerr := u.sessions.RevokeSession(ctx, info.TenantID, info.SessionID, "refresh_reuse_detected"); rerr == nil {
			_, _ = u.refresh.RevokeRefreshBySessions(ctx, []string{info.SessionID})
		}
		// THE alert. This event means a refresh token was stolen; the audit
		// pipeline routes action=token.reuse_detected to paging.
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID:  info.TenantID,
			ActorKind: "identity",
			SessionID: info.SessionID,
			Action:    "token.reuse_detected",
			Result:    "deny",
			IP:        authctx.ClientIP(ctx),
			Detail:    jsonx.Must(map[string]string{"family_id": info.FamilyID}),
		})
		return apperr.ErrRefreshReuse
	default:
		// Row exists with status=active yet the claim failed: expired.
		return apperr.ErrRefreshInvalid
	}
}
