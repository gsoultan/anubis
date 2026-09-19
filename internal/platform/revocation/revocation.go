// Package revocation carries "this stopped being valid" from wherever it is
// noticed to whoever is streaming it.
//
// It lives in platform rather than in gate or auth because both ends need it
// and neither owns it: the gate NOTICES revocations (it rebuilds the snapshot
// that contains them) and auth SERVES them (TokenService.StreamRevocations).
// Putting the type in either context would make the other import across a
// boundary that context-boundary.sh exists to keep.
package revocation

import (
	"sync"
	"time"
)

// Event is one thing that stopped being valid.
type Event struct {
	Kind       string // "session" | "epoch" | "synced"
	TenantSlug string
	SessionID  string
	IdentityID string
	Epoch      int
	ObservedAt time.Time
}

// Kinds an Event can carry.
const (
	KindSession = "session"
	KindEpoch   = "epoch"
	KindSynced  = "synced"
)

// subBuffer is how far a subscriber may fall behind before it starts losing
// events.
//
// Small on purpose. The stream is a cache invalidation, not an authorization
// decision — a consumer that misses one is still correct because it checks
// again and because tokens are short-lived — so buffering deeply would trade
// memory for a guarantee this design does not make. What it MUST NOT do is
// let one slow reader hold up the snapshot swap, which is on the gate's
// refresh path.
const subBuffer = 64

// Broker turns snapshot transitions into a per-tenant event stream.
//
// The source is the SNAPSHOT rather than the interactors that revoke, and
// that is the whole design. A revocation handled by one replica has to reach
// a consumer connected to another, and the snapshot is already the thing
// every replica rebuilds when the catalog version moves. Publishing from the
// interactor would deliver only to whichever instance served the request.
type Broker struct {
	mu   sync.RWMutex
	subs map[string]map[int64]chan Event
	next int64

	// dropped counts events a subscriber was too slow to take. Exported
	// through Dropped so an operator can see a consumer falling behind
	// rather than inferring it from missed revocations.
	dropped int64
}

func NewBroker() *Broker {
	return &Broker{subs: map[string]map[int64]chan Event{}}
}

// Subscribe returns a channel of this tenant's revocations and a function to
// stop. The channel is closed by the cancel function, never by the broker, so
// a reader cannot observe a close it did not ask for.
func (b *Broker) Subscribe(tenantSlug string) (<-chan Event, func()) {
	ch := make(chan Event, subBuffer)

	b.mu.Lock()
	b.next++
	id := b.next
	if b.subs[tenantSlug] == nil {
		b.subs[tenantSlug] = map[int64]chan Event{}
	}
	b.subs[tenantSlug][id] = ch
	b.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			if m := b.subs[tenantSlug]; m != nil {
				delete(m, id)
				if len(m) == 0 {
					delete(b.subs, tenantSlug)
				}
			}
			b.mu.Unlock()
			close(ch)
		})
	}
}

// Subscribers reports how many streams are open, for metrics and tests.
func (b *Broker) Subscribers() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := 0
	for _, m := range b.subs {
		n += len(m)
	}
	return n
}

// SubscribersFor reports how many streams are open for one tenant, so a
// producer can skip work nobody is waiting for.
func (b *Broker) SubscribersFor(tenantSlug string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[tenantSlug])
}

// Dropped reports events discarded because a subscriber was too slow.
func (b *Broker) Dropped() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.dropped
}

// Publish fans one event out to a tenant's subscribers.
//
// Never blocks: a full buffer drops the event and increments the counter.
// This runs on the snapshot refresh path, and a stream that could stall it
// would let one slow consumer stop the gate from seeing revocations at all —
// turning a delivery problem into an authorization one.
func (b *Broker) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[ev.TenantSlug] {
		select {
		case ch <- ev:
		default:
			b.dropped++
		}
	}
}
