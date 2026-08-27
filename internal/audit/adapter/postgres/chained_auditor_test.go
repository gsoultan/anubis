package auditpg

import (
	"context"
	"sync"
	"testing"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
)

// `authorize` is emitted on every decision and is allowed to drop rather than
// block. The drop itself was a bare `a.dropped++` on a field nothing read —
// executed on request goroutines, so a data race, and invisible either way.
//
// This fills the queue and hammers Emit from sixteen goroutines so the drop
// branch is taken concurrently. Under -race it fails on the old code.
func TestAuthorizeDropsAreCountedSafely(t *testing.T) {
	a := &ChainedAuditor{queue: make(chan auditdomain.AuditEvent, 1), done: make(chan struct{})}
	a.queue <- auditdomain.AuditEvent{} // full: every further authorize drops

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				a.Emit(context.Background(), auditdomain.AuditEvent{
					TenantID: auditdomain.InstallationTenant,
					Action:   "authorize", Result: "allow",
				})
			}
		}()
	}
	wg.Wait()
}

// An event with no tenant cannot be stored, and for a long time was thrown
// away before it was even counted — which is how the entire platform plane
// went unaudited. It must never be silent again.
func TestATenantlessEventIsCountedNotSwallowed(t *testing.T) {
	a := &ChainedAuditor{queue: make(chan auditdomain.AuditEvent, 4), done: make(chan struct{})}
	a.Emit(context.Background(), auditdomain.AuditEvent{Action: "platform.login", Result: "deny"})
	if len(a.queue) != 0 {
		t.Fatal("a tenant-less event was queued for a NOT NULL column")
	}
}
