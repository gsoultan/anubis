// Package tenancycache keeps the tenant a slug names in memory for a few
// seconds, for the request path.
package tenancycache

import (
	"context"
	"sync"
	"time"

	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// SlugCache answers TenantBySlug from memory for up to ttl, and delegates
// every other method untouched.
//
// Every request that carries a tenant token resolves its tenant by slug, and
// that lookup was a database round trip on each one: a third of the
// statements an allowed decision ran. A tenant's id and slug never change,
// so the only thing a cached answer can get wrong is its status, and ttl
// bounds how long a suspension takes to reach a token that is already out —
// ADR-0011 asks for exactly that, a short window rather than none.
//
// Only tenants that exist are kept, and never more than max of them: the
// slug can come from a request header, so a miss is never remembered and the
// map cannot be grown by asking for names that do not exist.
type SlugCache struct {
	tenancyport.TenantRepository
	clock clock.Clock
	ttl   time.Duration
	max   int

	mu      sync.RWMutex
	entries map[string]slugEntry
}

// New wraps next. A ttl of zero or less disables the cache entirely.
func New(next tenancyport.TenantRepository, c clock.Clock, ttl time.Duration, max int) *SlugCache {
	if max < 1 {
		max = 1
	}
	return &SlugCache{TenantRepository: next, clock: c, ttl: ttl, max: max,
		entries: make(map[string]slugEntry)}
}

func (c *SlugCache) TenantBySlug(ctx context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if c.ttl <= 0 {
		return c.TenantRepository.TenantBySlug(ctx, slug)
	}
	now := c.clock.Now()
	c.mu.RLock()
	e, ok := c.entries[slug]
	c.mu.RUnlock()
	if ok && now.Before(e.expires) {
		ref := e.ref // a copy: callers must not be able to edit the cache
		return &ref, nil
	}
	ref, err := c.TenantRepository.TenantBySlug(ctx, slug)
	if err != nil || ref == nil {
		return ref, err
	}
	c.mu.Lock()
	if _, present := c.entries[slug]; !present && len(c.entries) >= c.max {
		c.evictLocked(now)
	}
	c.entries[slug] = slugEntry{ref: *ref, expires: now.Add(c.ttl)}
	c.mu.Unlock()
	return ref, nil
}

// evictLocked makes room for one entry: every expired entry goes, and if
// none had expired, an arbitrary one does.
func (c *SlugCache) evictLocked(now time.Time) {
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) < c.max {
		return
	}
	for k := range c.entries {
		delete(c.entries, k)
		return
	}
}
