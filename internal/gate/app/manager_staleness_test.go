package gateapp

import (
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
)

// operations.md says: past ANUBIS_SNAPSHOT_MAX_AGE "the gate fails closed and
// readiness fails". Nothing tested it, and this is not a claim to leave
// unchecked — a snapshot that stayed "fresh" forever would serve
// authorization from arbitrarily old data, so revoked access would keep
// working for as long as the loader stayed broken. Fail-static is a feature;
// unbounded fail-static is the outage you do not notice.

func managerWith(maxAge time.Duration, ages map[string]time.Duration) *Manager {
	m := &Manager{maxAge: maxAge, data: map[string]*snapshot.Data{}}
	for slug, age := range ages {
		m.data[slug] = &snapshot.Data{LoadedAt: time.Now().Add(-age)}
	}
	return m
}

func TestASnapshotInsideMaxAgeIsServed(t *testing.T) {
	m := managerWith(5*time.Minute, map[string]time.Duration{"acme": 30 * time.Second})
	d, fresh := m.Get("acme")
	if d == nil || !fresh {
		t.Fatalf("a 30s-old snapshot was refused (data=%v fresh=%v)", d != nil, fresh)
	}
}

// The whole point: past the ceiling the answer stops being usable, even
// though the data is still in memory and would be cheap to serve.
func TestAStaleSnapshotIsNotFreshEnoughToServe(t *testing.T) {
	m := managerWith(5*time.Minute, map[string]time.Duration{"acme": 6 * time.Minute})
	d, fresh := m.Get("acme")
	if d == nil {
		t.Fatal("the snapshot was evicted; fail-static needs it present")
	}
	if fresh {
		t.Fatal("a 6-minute-old snapshot reported fresh under a 5-minute ceiling — " +
			"the gate would serve authorization from it")
	}
}

func TestAnUnknownTenantIsNeverFresh(t *testing.T) {
	m := managerWith(5*time.Minute, map[string]time.Duration{"acme": time.Second})
	if d, fresh := m.Get("nobody"); d != nil || fresh {
		t.Fatalf("an unloaded tenant served a snapshot (data=%v fresh=%v)", d != nil, fresh)
	}
}

// Readiness judges the WORST tenant, not the average: one stale tenant means
// this instance is failing closed for that tenant's traffic, and it should
// leave the load balancer rather than keep refusing them.
func TestReadinessJudgesTheWorstTenant(t *testing.T) {
	m := managerWith(5*time.Minute, map[string]time.Duration{
		"fresh-one": 10 * time.Second,
		"stale-one": 9 * time.Minute,
	})
	stale, age, loaded := m.Stale()
	if !loaded {
		t.Fatal("reported nothing loaded while holding two snapshots")
	}
	if !stale {
		t.Fatalf("one tenant 9 minutes stale and readiness said healthy (worst age %s)", age)
	}
	if age < 9*time.Minute {
		t.Fatalf("worst age reported as %s, want at least 9m — an average would hide it", age)
	}
}

// Before the first load there is nothing to serve and nothing to be stale.
// Both facts matter: readiness must fail, and it must not claim staleness it
// cannot measure.
func TestBeforeTheFirstLoadReadinessFails(t *testing.T) {
	m := managerWith(5*time.Minute, nil)
	stale, _, loaded := m.Stale()
	if loaded {
		t.Fatal("reported a load that never happened")
	}
	if !stale {
		t.Fatal("an instance with no snapshot at all reported ready")
	}
}

// A zero or negative ceiling is a misconfiguration that must not mean
// "never stale". Defaulting to five minutes is the safe reading.
func TestAnAbsentCeilingDoesNotMeanNoCeiling(t *testing.T) {
	m := NewManager(nil, nil, 0, nil)
	if m.maxAge != 5*time.Minute {
		t.Fatalf("maxAge %s from a zero setting — 0 would make every snapshot stale, "+
			"and treating it as infinite would make none", m.maxAge)
	}
}
