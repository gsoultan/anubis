package gateapp

import (
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/revocation"
)

func drain(ch <-chan revocation.Event) []revocation.Event {
	var out []revocation.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
}

func TestDiffEmitsNewlyRevokedSessions(t *testing.T) {
	b := revocation.NewBroker()
	ch, stop := b.Subscribe("acme")
	defer stop()

	prev := &snapshot.Data{RevokedSessions: map[string]bool{"old": true}}
	next := &snapshot.Data{RevokedSessions: map[string]bool{"old": true, "new": true}}
	publishRevocations(b, "acme", prev, next)

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Kind != revocation.KindSession || got[0].SessionID != "new" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestDiffEmitsEpochBumps(t *testing.T) {
	b := revocation.NewBroker()
	ch, stop := b.Subscribe("acme")
	defer stop()

	prev := &snapshot.Data{Identities: map[string]snapshot.Identity{"u1": {TokenEpoch: 3}}}
	next := &snapshot.Data{Identities: map[string]snapshot.Identity{"u1": {TokenEpoch: 4}}}
	publishRevocations(b, "acme", prev, next)

	got := drain(ch)
	if len(got) != 1 || got[0].Kind != revocation.KindEpoch {
		t.Fatalf("got %+v", got)
	}
	if got[0].IdentityID != "u1" || got[0].Epoch != 4 {
		t.Fatalf("got %+v", got[0])
	}
}

// An epoch that appears to FALL came from a replica that lagged. Announcing
// it would tell consumers to trust tokens they were already told to reject.
func TestDiffIgnoresEpochGoingBackwards(t *testing.T) {
	b := revocation.NewBroker()
	ch, stop := b.Subscribe("acme")
	defer stop()

	prev := &snapshot.Data{Identities: map[string]snapshot.Identity{"u1": {TokenEpoch: 9}}}
	next := &snapshot.Data{Identities: map[string]snapshot.Identity{"u1": {TokenEpoch: 4}}}
	publishRevocations(b, "acme", prev, next)

	if got := drain(ch); len(got) != 0 {
		t.Fatalf("a backwards epoch was announced: %+v", got)
	}
}

// The first snapshot is pre-existing state, not news. Announcing all of it
// would flood every consumer on startup with revocations they already knew
// about — and those are enforced by the snapshot regardless.
func TestFirstSnapshotAnnouncesNothing(t *testing.T) {
	b := revocation.NewBroker()
	ch, stop := b.Subscribe("acme")
	defer stop()

	next := &snapshot.Data{
		RevokedSessions: map[string]bool{"a": true, "b": true},
		Identities:      map[string]snapshot.Identity{"u1": {TokenEpoch: 7}},
	}
	publishRevocations(b, "acme", nil, next)

	if got := drain(ch); len(got) != 0 {
		t.Fatalf("the first snapshot announced %d events", len(got))
	}
}

// An unchanged snapshot must be silent, or every poll would look like a
// revocation storm.
func TestUnchangedSnapshotIsSilent(t *testing.T) {
	b := revocation.NewBroker()
	ch, stop := b.Subscribe("acme")
	defer stop()

	d := &snapshot.Data{
		RevokedSessions: map[string]bool{"a": true},
		Identities:      map[string]snapshot.Identity{"u1": {TokenEpoch: 7}},
	}
	publishRevocations(b, "acme", d, d)

	if got := drain(ch); len(got) != 0 {
		t.Fatalf("an unchanged snapshot announced %+v", got)
	}
}

// With nobody listening the diff must not run at all: this is the common
// case and it is on the gate's refresh path.
func TestNoSubscribersSkipsTheDiff(t *testing.T) {
	b := revocation.NewBroker()
	prev := &snapshot.Data{RevokedSessions: map[string]bool{}}
	next := &snapshot.Data{RevokedSessions: map[string]bool{"new": true}}
	publishRevocations(b, "acme", prev, next)

	if b.Dropped() != 0 {
		t.Fatalf("events were published with no subscribers: dropped=%d", b.Dropped())
	}
}

func TestNilBrokerIsSafe(t *testing.T) {
	publishRevocations(nil, "acme",
		&snapshot.Data{RevokedSessions: map[string]bool{}},
		&snapshot.Data{RevokedSessions: map[string]bool{"x": true}})
}
