// Package ratelimit is a sharded in-memory token-bucket limiter keyed by
// attacker-supplied strings (ip:..., acct:..., tenant:...).
//
// Standing rule: any map keyed by attacker-supplied input needs a BOUND and
// an EVICTION. Each shard caps its entries; when full, the stalest entries
// are evicted. An attacker rotating keys can therefore reset *other keys'*
// counters at worst, never grow memory without bound — and the per-account
// limit (the one that stops credential stuffing) keys on the *target*
// account, which the attacker cannot rotate.
//
// Postgres never sees attack traffic (counters live here); Redis arrives by
// ADR only when multi-instance deployment needs shared counters.
package ratelimit

import (
	"hash/fnv"
	"time"
)

const (
	shardCount  = 16
	maxPerShard = 8192 // 16 shards x 8192 ≈ 131k tracked keys
	evictBatch  = 256
)

// Limiter is safe for concurrent use.
type Limiter struct {
	shards [shardCount]*shard
	now    func() time.Time
}

func New() *Limiter {
	l := &Limiter{now: time.Now}
	for i := range l.shards {
		l.shards[i] = &shard{m: make(map[string]*bucket)}
	}
	return l
}

// NewWithClock is the test seam.
func NewWithClock(now func() time.Time) *Limiter {
	l := New()
	l.now = now
	return l
}

// Allow consumes one token from key's bucket under lim. It reports whether
// the request may proceed and, when denied, how long until a token appears.
func (l *Limiter) Allow(key string, lim Limit) (ok bool, retryAfter time.Duration) {
	if lim.PerMinute <= 0 {
		return true, 0
	}
	now := l.now()
	sh := l.shards[shardFor(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()

	b := sh.m[key]
	if b == nil {
		if len(sh.m) >= maxPerShard {
			evictStalest(sh)
		}
		b = &bucket{tokens: lim.Burst, last: now}
		sh.m[key] = b
	}
	perSec := lim.PerMinute / 60.0
	b.tokens += now.Sub(b.last).Seconds() * perSec
	if b.tokens > lim.Burst {
		b.tokens = lim.Burst
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	need := (1 - b.tokens) / perSec
	return false, time.Duration(need * float64(time.Second))
}

// AllowAll consumes across several axes; the first denial wins. Axes that
// already consumed are NOT refunded — a denied request still spent attempts
// on the axes before it, which is the conservative direction.
func (l *Limiter) AllowAll(pairs ...KeyLimit) (ok bool, retryAfter time.Duration) {
	for _, p := range pairs {
		if p.Key == "" {
			continue
		}
		if ok, ra := l.Allow(p.Key, p.Limit); !ok {
			return false, ra
		}
	}
	return true, 0
}

func shardFor(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % shardCount)
}

// evictStalest removes the evictBatch least-recently-used entries. O(n) scan,
// amortised: it runs only when a full shard meets a new key.
func evictStalest(sh *shard) {
	type cand struct {
		key  string
		last time.Time
	}
	victims := make([]cand, 0, evictBatch)
	worstOf := func() int {
		newest, at := 0, victims[0].last
		for i, v := range victims[1:] {
			if v.last.After(at) {
				newest, at = i+1, v.last
			}
		}
		return newest
	}
	for k, b := range sh.m {
		if len(victims) < evictBatch {
			victims = append(victims, cand{k, b.last})
			continue
		}
		w := worstOf()
		if b.last.Before(victims[w].last) {
			victims[w] = cand{k, b.last}
		}
	}
	for _, v := range victims {
		delete(sh.m, v.key)
	}
}
