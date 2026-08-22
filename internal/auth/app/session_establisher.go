package authapp

import (
	"context"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/txm"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

type SessionEstablisher struct {
	sessions authport.SessionRepository
	ids      identityport.IdentityRepository
	issuer   TokenIssuer
	tx       txm.TxManager
	clock    clock.Clock
}

func NewSessionEstablisher(
	sessions authport.SessionRepository,
	ids identityport.IdentityRepository,
	issuer TokenIssuer,
	tx txm.TxManager,
	clock clock.Clock,
) *SessionEstablisher {
	return &SessionEstablisher{sessions: sessions, ids: ids, issuer: issuer, tx: tx, clock: clock}
}

func (e *SessionEstablisher) Establish(ctx context.Context, tenant *tenancydomain.TenantRef, realm *identitydomain.Realm, identityID, clientID, deviceFP string, amr []string) (*TokenPair, error) {
	var pair *TokenPair
	err := e.tx.WithinTx(ctx, func(ctx context.Context) error {
		sess, err := e.sessions.CreateSession(ctx, authdomain.SessionInput{
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
			return apperr.ErrInternal.Wrap(err)
		}
		view, err := e.sessions.SessionLive(ctx, sess.ID)
		if err != nil {
			return apperr.ErrInternal.Wrap(err)
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
