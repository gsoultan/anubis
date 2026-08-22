package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// logoutInteractor implements all three sign-out shapes; each narrow usecase
// interface exposes exactly one of them.
type logoutInteractor struct {
	sessions    repository.SessionRepository
	refresh     repository.RefreshRepository
	ids         repository.IdentityRepository
	tx          repository.TxManager
	audit       repository.Auditor
	backchannel *backchannelLogout
}

func NewLogoutInteractor(
	sessions repository.SessionRepository,
	refresh repository.RefreshRepository,
	ids repository.IdentityRepository,
	tx repository.TxManager,
	audit repository.Auditor,
	backchannel *backchannelLogout,
) *logoutInteractor {
	return &logoutInteractor{
		sessions: sessions, refresh: refresh, ids: ids,
		tx: tx, audit: audit, backchannel: backchannel,
	}
}

// Logout — one device.
func (u *logoutInteractor) Execute(ctx context.Context) error {
	p, ok := authctx.From(ctx)
	if !ok || p.SessionID == "" {
		return domain.ErrUnauthenticated
	}
	return u.revokeOne(ctx, p, p.SessionID, "logout")
}

// LogoutSession — one named device, owned by the caller.
type logoutSessionInteractor struct{ *logoutInteractor }

func (u *logoutInteractor) Session() LogoutSessionUsecase {
	return &logoutSessionInteractor{u}
}

func (u *logoutSessionInteractor) Execute(ctx context.Context, sessionID string) error {
	p, ok := authctx.From(ctx)
	if !ok {
		return domain.ErrUnauthenticated
	}
	return u.revokeOne(ctx, p, sessionID, "logout_session")
}

// LogoutAll — every device + epoch bump + back-channel.
type logoutAllInteractor struct{ *logoutInteractor }

func (u *logoutInteractor) All() LogoutAllUsecase {
	return &logoutAllInteractor{u}
}

func (u *logoutAllInteractor) Execute(ctx context.Context) (int, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return 0, domain.ErrUnauthenticated
	}
	var revoked []repository.RevokedSession
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		revoked, err = u.sessions.RevokeAllSessions(ctx, p.TenantID, p.IdentityID, "logout_all")
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		ids := make([]string, len(revoked))
		for i, s := range revoked {
			ids[i] = s.ID
		}
		if len(ids) > 0 {
			if _, err := u.refresh.RevokeRefreshBySessions(ctx, ids); err != nil {
				return domain.ErrInternal.Wrap(err)
			}
		}
		if _, err := u.ids.BumpTokenEpoch(ctx, p.TenantID, p.IdentityID); err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, Action: "auth.logout_all", Result: "allow",
		IP:     authctx.ClientIP(ctx),
		Detail: mustJSON(map[string]int{"sessions_revoked": len(revoked)}),
	})
	// After commit: notify every application that registered a
	// backchannel_logout_uri, so app-local cookies die with the SSO session.
	u.backchannel.NotifyAll(ctx, p.TenantID, p.IdentityID, revoked)
	return len(revoked), nil
}

func (u *logoutInteractor) revokeOne(ctx context.Context, p *authctx.Principal, sessionID, reason string) error {
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		rs, err := u.sessions.RevokeSession(ctx, p.TenantID, sessionID, reason)
		if err != nil {
			return domain.ErrNotFound
		}
		// Ownership: you sign out YOUR sessions. Admin deprovisioning uses
		// the admin plane, which is permission-gated.
		if rs.IdentityID != p.IdentityID {
			return domain.ErrPermissionDenied
		}
		_, err = u.refresh.RevokeRefreshBySessions(ctx, []string{sessionID})
		return err
	})
	if err != nil {
		return err
	}
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: sessionID, Action: "auth." + reason, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: []byte("{}"),
	})
	return nil
}
