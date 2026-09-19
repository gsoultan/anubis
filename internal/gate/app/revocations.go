package gateapp

import (
	"time"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/revocation"
)

// publishRevocations announces what changed between two snapshots of one
// tenant.
//
// The SOURCE is the snapshot, not the interactors that revoke, and that is
// the whole design. A revocation handled by one replica has to reach a
// consumer connected to another, and the snapshot is already the thing every
// replica rebuilds when the catalog version moves. Publishing from the
// interactor would deliver only to whichever instance served the request.
//
// prev nil means this is the tenant's first snapshot: everything in it is
// pre-existing state rather than news. Announcing a process's whole backlog
// on startup would flood every consumer with revocations they already knew
// about, and the ones that matter are enforced by the snapshot regardless.
func publishRevocations(b *revocation.Broker, slug string, prev, next *snapshot.Data) {
	if b == nil || next == nil || prev == nil {
		return
	}
	// Nobody listening: skip the comparison. This is on the refresh path and
	// zero subscribers is the common case.
	if b.SubscribersFor(slug) == 0 {
		return
	}

	now := time.Now()
	for sid := range next.RevokedSessions {
		if !prev.RevokedSessions[sid] {
			b.Publish(revocation.Event{
				Kind: revocation.KindSession, TenantSlug: slug,
				SessionID: sid, ObservedAt: now,
			})
		}
	}
	for id, cur := range next.Identities {
		// Only an INCREASE. An epoch that appears to fall came from a
		// replica that lagged, and re-announcing it would tell consumers to
		// trust tokens they had already been told to reject.
		if old, ok := prev.Identities[id]; ok && cur.TokenEpoch > old.TokenEpoch {
			b.Publish(revocation.Event{
				Kind: revocation.KindEpoch, TenantSlug: slug,
				IdentityID: id, Epoch: cur.TokenEpoch, ObservedAt: now,
			})
		}
	}
}
