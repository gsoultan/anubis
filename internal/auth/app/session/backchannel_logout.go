package sessionapp

import (
	"context"
	"encoding/json"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// backchannelLogout signs logout tokens and hands them to the notifier.
// Delivery is best-effort and asynchronous — a slow application must not
// hold the user's logout hostage — but every failure is logged by the
// notifier implementation.
type backchannelLogout struct {
	issuer   string
	ring     *keyring.Manager
	apps     tenancyport.BackchannelDirectoryRepository
	notifier authport.BackchannelNotifier
	clock    clock.Clock
}

func NewBackchannelLogout(
	issuer string,
	ring *keyring.Manager,
	apps tenancyport.BackchannelDirectoryRepository,
	notifier authport.BackchannelNotifier,
	clock clock.Clock,
) *backchannelLogout {
	return &backchannelLogout{issuer: issuer, ring: ring, apps: apps, notifier: notifier, clock: clock}
}

func (b *backchannelLogout) NotifyAll(ctx context.Context, tenantID, identityID string, sessions []authdomain.RevokedSession) {
	slugs, uris, err := b.apps.BackchannelApps(ctx, tenantID)
	if err != nil || len(uris) == 0 {
		return
	}
	sid := ""
	if len(sessions) > 0 {
		sid = sessions[0].ID
	}
	for i, uri := range uris {
		token, err := b.mint(slugs[i], identityID, sid)
		if err != nil {
			continue
		}
		b.notifier.NotifyLogout(ctx, uri, token)
	}
}

// mint builds an OIDC-back-channel-shaped logout token as v4.public PASETO.
func (b *backchannelLogout) mint(audience, sub, sid string) (string, error) {
	key, err := b.ring.Ring().ActiveAccess()
	if err != nil {
		return "", err
	}
	jti, err := secret.New(16)
	if err != nil {
		return "", err
	}
	now := b.clock.Now()
	body, err := json.Marshal(map[string]any{
		"iss": b.issuer,
		"aud": []string{audience},
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(),
		"jti": jti,
		"sub": sub,
		"sid": sid,
		"events": map[string]any{
			"http://schemas.openid.net/event/backchannel-logout": map[string]any{},
		},
	})
	if err != nil {
		return "", err
	}
	footer, _ := json.Marshal(map[string]string{"kid": key.Kid})
	return paseto.Sign(key.Private, body, footer, nil)
}
