package revocation

import (
	"sync"
	"testing"
	"time"
)

func TestSubscriberReceivesItsOwnTenantOnly(t *testing.T) {
	b := NewBroker()
	acme, stopA := b.Subscribe("acme")
	defer stopA()
	other, stopB := b.Subscribe("other")
	defer stopB()

	b.Publish(Event{Kind: KindSession, TenantSlug: "acme", SessionID: "s1"})

	select {
	case ev := <-acme:
		if ev.SessionID != "s1" {
			t.Fatalf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("the tenant's own subscriber got nothing")
	}
	select {
	case ev := <-other:
		t.Fatalf("another tenant's subscriber received %+v", ev)
	default:
	}
}

// THE property the gate depends on. Publish runs on the snapshot refresh
// path; a reader that has stopped reading must not be able to stall it, or
// one wedged consumer stops every tenant's snapshot from being announced.
func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := NewBroker()
	_, stop := b.Subscribe("acme") // never read
	defer stop()

	done := make(chan struct{})
	go func() {
		for i := 0; i < subBuffer*4; i++ {
			b.Publish(Event{Kind: KindSession, TenantSlug: "acme", SessionID: "s"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a subscriber that stopped reading")
	}
	if b.Dropped() == 0 {
		t.Fatal("nothing was reported dropped, so the overflow went unnoticed")
	}
}

// Cancelling must both stop delivery and close the channel, so a reader's
// range loop ends rather than hanging forever.
func TestCancelClosesAndDeregisters(t *testing.T) {
	b := NewBroker()
	ch, stop := b.Subscribe("acme")
	if b.Subscribers() != 1 {
		t.Fatalf("subscribers=%d", b.Subscribers())
	}
	stop()
	if b.Subscribers() != 0 {
		t.Fatalf("still registered after cancel: %d", b.Subscribers())
	}
	if _, open := <-ch; open {
		t.Fatal("the channel was not closed")
	}
	stop() // must be idempotent; a deferred cancel after an explicit one
}

func TestSubscribersForIsPerTenant(t *testing.T) {
	b := NewBroker()
	_, a1 := b.Subscribe("acme")
	defer a1()
	_, a2 := b.Subscribe("acme")
	defer a2()
	_, o1 := b.Subscribe("other")
	defer o1()

	if got := b.SubscribersFor("acme"); got != 2 {
		t.Fatalf("acme=%d, want 2", got)
	}
	if got := b.SubscribersFor("nobody"); got != 0 {
		t.Fatalf("nobody=%d, want 0", got)
	}
}

// Subscribe and Publish run concurrently in production: a stream opens while
// the refresh path is mid-fan-out.
func TestConcurrentSubscribeAndPublish(t *testing.T) {
	b := NewBroker()
	var wg sync.WaitGroup
	stopAll := make(chan struct{})

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopAll:
					return
				default:
				}
				ch, stop := b.Subscribe("acme")
				go func() {
					for range ch {
					}
				}()
				stop()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopAll:
					return
				default:
				}
				b.Publish(Event{Kind: KindEpoch, TenantSlug: "acme"})
			}
		}()
	}
	time.Sleep(200 * time.Millisecond)
	close(stopAll)
	wg.Wait()
}
