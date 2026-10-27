package authzshare

import (
	"context"
	"encoding/json"
	"strings"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/authz/domain/share"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	authzshareport "github.com/gsoultan/anubis/internal/authz/port/share"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/validate"
)

const maxNote = 500

type shareInteractor struct {
	shares authzshareport.ShareRepository
	authz  authzport.AuthzRepository
	ids    identityport.IdentityRepository
	clock  clock.Clock
	audit  auditport.Auditor
}

func NewShareInteractor(
	shares authzshareport.ShareRepository,
	authz authzport.AuthzRepository,
	ids identityport.IdentityRepository,
	clk clock.Clock,
	audit auditport.Auditor,
) ShareUsecase {
	return &shareInteractor{shares: shares, authz: authz, ids: ids, clock: clk, audit: audit}
}

// person is the caller as a tenant person. Operators and services do not
// share: a share is one person's access handed to another, and neither
// holds access as a person.
func person(ctx context.Context) (*authctx.Principal, error) {
	p, ok := authctx.From(ctx)
	if !ok || p.IdentityID == "" || p.Platform || p.Service {
		return nil, apperr.ErrUnauthenticated
	}
	return p, nil
}

func (u *shareInteractor) Share(ctx context.Context, in ShareInput) (*share.Share, error) {
	p, err := person(ctx)
	if err != nil {
		return nil, err
	}
	record, err := share.RecordKey(in.RecordKind, in.RecordRef)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.With("record", err.Error())
	}
	if in.ToIdentityID == "" || in.ToIdentityID == p.IdentityID {
		return nil, apperr.ErrInvalidArgument.With("to", "name somebody other than yourself")
	}
	if len(in.Permissions) == 0 || len(in.Permissions) > share.MaxPermissions {
		return nil, apperr.ErrInvalidArgument.With("permissions", "between 1 and 32 permission keys")
	}
	now := u.clock.Now()
	switch {
	case in.ValidUntil.IsZero() || !in.ValidUntil.After(now):
		return nil, apperr.ErrInvalidArgument.With("valid_until", "must be in the future; a share always ends")
	case in.ValidUntil.After(now.Add(share.MaxLife)):
		return nil, apperr.ErrInvalidArgument.With("valid_until", "a share may last at most 90 days")
	}
	if len(in.Note) > maxNote {
		return nil, apperr.ErrInvalidArgument.With("note", "at most 500 characters")
	}
	scopes, err := recordScopes(in.RecordScopes)
	if err != nil {
		return nil, err
	}

	// The recipient must be somebody in this tenant who can sign in. A share
	// to a disabled account is not refused at decision time only — it is
	// refused here, where the person making it can be told.
	to, err := u.ids.Identity(ctx, p.TenantID, in.ToIdentityID)
	if err != nil || to == nil {
		return nil, apperr.ErrNotFound.With("to", in.ToIdentityID)
	}
	if aerr := to.CanAuthenticate(); aerr != nil {
		return nil, apperr.ErrInvalidArgument.With("to", "that account cannot sign in")
	}

	// You can only share what you can do: every permission, at the record's
	// own places, through the sharer's GRANTS. "_record" is stripped above,
	// so a share cannot beget a share.
	targets := jsonx.Must(scopes)
	perms := dedupe(in.Permissions)
	for _, key := range perms {
		meta, err := u.authz.PermissionByKey(ctx, p.TenantID, key)
		if err != nil || meta == nil {
			return nil, apperr.ErrInvalidArgument.With("permission", key).With("hint", "unknown permission")
		}
		allowed, err := u.authz.Authorize(ctx, p.IdentityID, p.TenantID, key, targets)
		if err != nil {
			return nil, apperr.ErrInternal.Wrap(err)
		}
		if !allowed {
			u.emit(ctx, p, "share.create", in.ToIdentityID, "deny", map[string]string{
				"record": record, "permission": key, "reason": "sharer_not_authorized",
			})
			return nil, apperr.ErrPermissionDenied.With("permission", key).
				With("hint", "you can only share what you can do")
		}
	}

	sh, err := u.shares.CreateShare(ctx, share.CreateInput{
		TenantID: p.TenantID, FromIdentityID: p.IdentityID, ToIdentityID: in.ToIdentityID,
		Record: record, RecordScopes: scopes, Permissions: perms,
		Note: strings.TrimSpace(in.Note), ValidUntil: in.ValidUntil,
	})
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "share.create", in.ToIdentityID, "allow", map[string]string{
		"share_id": sh.ID, "record": record, "permissions": strings.Join(perms, ","),
		"valid_until": sh.ValidUntil.UTC().Format("2006-01-02T15:04:05Z"),
	})
	return sh, nil
}

func (u *shareInteractor) Unshare(ctx context.Context, shareID, reason string) error {
	p, err := person(ctx)
	if err != nil {
		return err
	}
	sh, err := u.shares.Share(ctx, p.TenantID, shareID)
	if err != nil {
		return apperr.ErrNotFound
	}
	if sh.FromIdentityID != p.IdentityID {
		if ok, err := u.holdsAll(ctx, p, sh); err != nil {
			return err
		} else if !ok {
			return apperr.ErrPermissionDenied.With("hint", "only the sharer, or somebody who holds everything it gives, may end a share")
		}
	}
	if _, err := u.shares.RevokeShare(ctx, p.TenantID, shareID, strings.TrimSpace(reason)); err != nil {
		return err
	}
	u.emit(ctx, p, "share.revoke", sh.ToIdentityID, "allow", map[string]string{
		"share_id": sh.ID, "record": sh.Record, "reason": strings.TrimSpace(reason),
	})
	return nil
}

// LinkPrefix is the auth context's accesslink.LinkPrefix, repeated here
// rather than imported: authz must not depend on auth's application layer.
const LinkPrefix = "anb_lk_"

func (u *shareInteractor) CreateAccessLink(ctx context.Context, shareID string) (string, error) {
	p, err := person(ctx)
	if err != nil {
		return "", err
	}
	sh, err := u.shares.Share(ctx, p.TenantID, shareID)
	if err != nil {
		return "", apperr.ErrNotFound
	}
	if !sh.Live(u.clock.Now()) {
		return "", apperr.ErrInvalidArgument.With("share", "ended")
	}
	if sh.FromIdentityID != p.IdentityID {
		if ok, err := u.holdsAll(ctx, p, sh); err != nil {
			return "", err
		} else if !ok {
			return "", apperr.ErrPermissionDenied.With("hint", "only the sharer, or somebody who holds everything it gives, may make a link")
		}
	}
	raw, err := secret.New(32)
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	token := LinkPrefix + raw
	ok, err := u.shares.SetShareLink(ctx, p.TenantID, sh.ID, secret.Hash(token))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", apperr.ErrInvalidArgument.With("share", "ended")
	}
	u.emit(ctx, p, "share.link_create", sh.ToIdentityID, "allow", map[string]string{
		"share_id": sh.ID, "record": sh.Record,
	})
	return token, nil
}

func (u *shareInteractor) SharesOnRecord(ctx context.Context, kind, ref string, scopes map[string]string) ([]share.Share, error) {
	p, err := person(ctx)
	if err != nil {
		return nil, err
	}
	record, err := share.RecordKey(kind, ref)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.With("record", err.Error())
	}
	all, err := u.shares.SharesOnRecord(ctx, p.TenantID, record)
	if err != nil {
		return nil, err
	}
	// Visibility follows access: a share is visible to its two parties, and
	// to anybody who could have made it themselves.
	out := make([]share.Share, 0, len(all))
	for i := range all {
		sh := all[i]
		if sh.FromIdentityID == p.IdentityID || sh.ToIdentityID == p.IdentityID {
			out = append(out, sh)
			continue
		}
		probe := sh
		if len(scopes) > 0 {
			if clean, err := recordScopes(scopes); err == nil {
				probe.RecordScopes = clean
			}
		}
		if ok, err := u.holdsAll(ctx, p, &probe); err != nil {
			return nil, err
		} else if ok {
			out = append(out, sh)
		}
	}
	return out, nil
}

func (u *shareInteractor) SharedWithMe(ctx context.Context) ([]share.Share, error) {
	p, err := person(ctx)
	if err != nil {
		return nil, err
	}
	return u.shares.SharesForRecipient(ctx, p.TenantID, p.IdentityID)
}

// holdsAll asks whether the caller holds, through grants, every permission
// the share confers at the places it was made for.
func (u *shareInteractor) holdsAll(ctx context.Context, p *authctx.Principal, sh *share.Share) (bool, error) {
	targets, err := json.Marshal(sh.RecordScopes)
	if err != nil {
		return false, apperr.ErrInternal.Wrap(err)
	}
	for _, key := range sh.Permissions {
		ok, err := u.authz.Authorize(ctx, p.IdentityID, p.TenantID, key, targets)
		if err != nil {
			return false, apperr.ErrInternal.Wrap(err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// recordScopes validates the places and strips "_record": the sharer is
// authorised through grants, never through a share of their own.
func recordScopes(in map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(in))
	if len(in) > 16 {
		return nil, apperr.ErrInvalidArgument.With("record_scopes", "too many axes")
	}
	for axis, node := range in {
		if axis == "_record" {
			continue
		}
		if axis != "_owner" && !validate.ValidCode(axis) {
			return nil, apperr.ErrInvalidArgument.With("axis", axis)
		}
		if node == "" {
			return nil, apperr.ErrInvalidArgument.With("axis", axis).With("hint", "empty target")
		}
		out[axis] = node
	}
	return out, nil
}

func dedupe(keys []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

func (u *shareInteractor) emit(ctx context.Context, p *authctx.Principal, action, target, result string, detail map[string]string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: result,
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	})
}
