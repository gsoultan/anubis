// Package authzshare is record sharing: one person opening one record to
// another for a while, with a subset of what they themselves may do on it.
//
// The rules, each of which the system being replaced got wrong:
//
//   - You can only share what you can do. Every permission handed over is
//     authorised for the sharer, at the record's own places, before the
//     share exists.
//   - The share is bound to the record. It confers nothing anywhere else.
//   - It always ends, and revoking it ends it now — for the recipient's
//     next request, through the gate snapshot.
//   - A share is never a way in. It is consulted by authorize() for a
//     signed-in recipient; it does not sign anybody in.
package authzshare

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/share"
)

// ShareUsecase is the self-service sharing surface (proto ShareService).
type ShareUsecase interface {
	Share(ctx context.Context, in ShareInput) (*share.Share, error)
	// Unshare revokes. The sharer may always; anybody else needs every
	// permission the share confers, at the record's places.
	Unshare(ctx context.Context, shareID, reason string) error
	// SharesOnRecord lists live shares on one record that the caller may
	// see: those they made, those made to them, and those whose every
	// permission they hold at the record's places.
	SharesOnRecord(ctx context.Context, kind, ref string, scopes map[string]string) ([]share.Share, error)
	// SharedWithMe lists live shares to the caller.
	SharedWithMe(ctx context.Context) ([]share.Share, error)
	// CreateAccessLink mints the link a recipient with no password opens
	// the record with (AuthService/OpenAccessLink). Returned once; making
	// another replaces it. The sharer may; so may anybody who could end
	// the share. Anubis mails nothing — the application owns the record's
	// URL and the message, and puts the token where it belongs.
	CreateAccessLink(ctx context.Context, shareID string) (string, error)
}
