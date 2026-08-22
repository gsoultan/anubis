package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// sessionEstablisher creates the session row and mints tokens in one
// transaction — the shared tail of password login, MFA verify and device
// verify.
type sessionEstablisher struct {
	sessions repository.SessionRepository
	ids      repository.IdentityRepository
	issuer   TokenIssuer
	tx       repository.TxManager
	clock    repository.Clock
}

func newSessionEstablisher(
	sessions repository.SessionRepository,
	ids repository.IdentityRepository,
	issuer TokenIssuer,
	tx repository.TxManager,
	clock repository.Clock,
) *sessionEstablisher {
	return &sessionEstablisher{sessions: sessions, ids: ids, issuer: issuer, tx: tx, clock: clock}
}

func (e *sessionEstablisher) establish(ctx context.Context, tenant *repository.TenantRef, realm *domain.Realm, identityID, clientID, deviceFP string, amr []string) (*TokenPair, error) {
	var pair *TokenPair
	err := e.tx.WithinTx(ctx, func(ctx context.Context) error {
		sess, err := e.sessions.CreateSession(ctx, repository.SessionInput{
			IdentityID:   identityID,
			TenantID:     tenant.ID,
			AMR:          amr,
			DeviceFP:     deviceFP,
			IP:           authctx.ClientIP(ctx),
			UserAgent:    authctx.UserAgent(ctx),
			ActiveScopes: []byte("{}"),
			ExpiresAt:    e.clock.Now().Add(realm.SessionTTL),
		})
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		view, err := e.sessions.SessionLive(ctx, sess.ID)
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		pair, err = e.issuer.Issue(ctx, IssueInput{
			Session:    view,
			TenantSlug: tenant.Slug,
			ClientID:   clientID,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	e.ids.TouchLastLogin(ctx, identityID)
	return pair, nil
}
