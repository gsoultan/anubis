package controlapp

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	controlport "github.com/gsoultan/anubis/internal/control/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
)

// PlatformAPIKeyUsecase mints and revokes operators' machine credentials.
//
// Migration 0029 made administration operator-only, which killed the one
// automated path that mattered: applying a manifest from CI. This gives it
// back without giving anything else away — a key acts AS its owner and
// carries exactly their assignments, checked on every call.
type PlatformAPIKeyUsecase interface {
	// CreateAPIKey returns the full key ONCE. It is never stored and cannot
	// be shown again.
	CreateAPIKey(ctx context.Context, in CreateAPIKeyInput) (fullKey string, rec *controldomain.PlatformAPIKey, err error)
	ListAPIKeys(ctx context.Context) ([]controldomain.PlatformAPIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
}

// CreateAPIKeyInput describes the credential to mint.
type CreateAPIKeyInput struct {
	// OwnerID is the operator the key acts as. Empty means the caller.
	OwnerID string
	Label   string
	// ExpiresIn is required and bounded: a credential that administers the
	// installation must not outlive the reason it was made.
	ExpiresIn time.Duration
}

// maxAPIKeyLife bounds a machine credential. Long enough not to be a weekly
// chore, short enough that a forgotten key stops working within a quarter.
const maxAPIKeyLife = 90 * 24 * time.Hour

type apiKeyInteractor struct {
	guard platformGuard
	keys  controlport.PlatformAPIKeyStore
	users controlport.PlatformUserStore
	clock clock.Clock
	audit auditport.Auditor
}

func NewPlatformAPIKeyInteractor(
	keys controlport.PlatformAPIKeyStore,
	users controlport.PlatformUserStore,
	read controlport.AssignmentReader,
	clk clock.Clock,
	audit auditport.Auditor,
) PlatformAPIKeyUsecase {
	return &apiKeyInteractor{
		guard: platformGuard{read: read, clock: clk},
		keys:  keys, users: users, clock: clk, audit: audit,
	}
}

// CreateAPIKey mints a credential. Gated on PermAssignOperators — the same
// authority as appointing an operator, because that is what this is: a key
// is another way for its owner's authority to act, and handing one out
// deserves the same permission as handing out the authority itself.
func (u *apiKeyInteractor) CreateAPIKey(ctx context.Context, in CreateAPIKeyInput) (string, *controldomain.PlatformAPIKey, error) {
	p, _, err := u.guard.require(ctx, controldomain.PermAssignOperators)
	if err != nil {
		return "", nil, err
	}
	owner := strings.TrimSpace(in.OwnerID)
	if owner == "" {
		owner = p.IdentityID
	}
	who, _, err := u.users.PlatformUserByID(ctx, owner)
	if err != nil {
		return "", nil, err
	}
	if who == nil || !who.Active() {
		// A key for a disabled operator would authenticate to nothing; say so
		// now rather than hand over a credential that cannot work.
		return "", nil, apperr.ErrInvalidArgument.With("owner", "no such active operator")
	}
	life := in.ExpiresIn
	if life <= 0 || life > maxAPIKeyLife {
		life = maxAPIKeyLife
	}
	full, lookup, hash, err := secret.NewAPIKey()
	if err != nil {
		return "", nil, apperr.ErrInternal.Wrap(err)
	}
	expires := u.clock.Now().Add(life)
	id, err := u.keys.CreatePlatformAPIKey(ctx, who.ID, strings.TrimSpace(in.Label),
		lookup, hex.EncodeToString(hash), p.IdentityID, expires)
	if err != nil {
		return "", nil, err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		ActorID: p.IdentityID, ActorKind: "platform_user", TargetID: id,
		Action: "platform.apikey.create", Result: "allow", IP: authctx.ClientIP(ctx),
	})
	return full, &controldomain.PlatformAPIKey{
		ID: id, PlatformUserID: who.ID, Username: who.Username,
		Label: in.Label, Lookup: lookup, CreatedAt: u.clock.Now(), ExpiresAt: expires,
	}, nil
}

func (u *apiKeyInteractor) ListAPIKeys(ctx context.Context) ([]controldomain.PlatformAPIKey, error) {
	if _, _, err := u.guard.require(ctx, controldomain.PermAssignOperators); err != nil {
		return nil, err
	}
	return u.keys.ListPlatformAPIKeys(ctx)
}

func (u *apiKeyInteractor) RevokeAPIKey(ctx context.Context, id string) error {
	p, _, err := u.guard.require(ctx, controldomain.PermAssignOperators)
	if err != nil {
		return err
	}
	if err := u.keys.RevokePlatformAPIKey(ctx, id); err != nil {
		return err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		ActorID: p.IdentityID, ActorKind: "platform_user", TargetID: id,
		Action: "platform.apikey.revoke", Result: "allow", IP: authctx.ClientIP(ctx),
	})
	return nil
}
