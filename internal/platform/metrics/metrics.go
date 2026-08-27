// Package metrics is the operational instrument panel: counters, gauges and
// latency histograms exposed in Prometheus text format. Hand-rolled on
// stdlib by design (ADR-0002) — the exposition format is a page of code,
// which is cheaper than a dependency tree with network access.
//
// Every label value here is CODE-DEFINED (endpoint names, error codes,
// audit actions, job names, tenant slugs from validated rows) — never raw
// caller input, so cardinality is bounded by the codebase, not by traffic.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// counters holds monotonically increasing series keyed by family + labels.
var counters sync.Map // string -> *atomic.Uint64

// gauges holds last-write-wins series (unix seconds or plain values).
var gauges sync.Map // string -> *atomic.Int64

// histograms holds latency distributions keyed by family + labels.
var histograms sync.Map // string -> *histogram

// poolStats, when registered, is read at scrape time — pool numbers are
// point-in-time by nature and polling them on a timer would only be staler.
var poolStats atomic.Pointer[func() PoolStats]

// buildInfo carries the version label; set once at boot.
var buildInfo atomic.Pointer[string]

// bucketBounds are seconds. The last implicit bucket is +Inf. Tuned to this
// system's budgets: authorize p95 < 2 ms lives in the first buckets, the
// KDF-dominated login (~50 ms) in the middle, timeouts at the tail.
var bucketBounds = [...]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type histogram struct {
	buckets [len(bucketBounds) + 1]atomic.Uint64
	sumUs   atomic.Uint64 // microseconds, converted at exposition
	count   atomic.Uint64
}

// key joins a family and its label values with a separator that cannot
// appear in code-defined identifiers.
func key(parts ...string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += "\xff" + p
	}
	return out
}

func counter(k string) *atomic.Uint64 {
	if v, ok := counters.Load(k); ok {
		return v.(*atomic.Uint64)
	}
	v, _ := counters.LoadOrStore(k, new(atomic.Uint64))
	return v.(*atomic.Uint64)
}

// IncEndpoint counts one RPC by endpoint and outcome code ("ok" or the
// stable apperr code). Rate-limit pressure, permission refusals and
// internal errors are all alertable from this one family.
func IncEndpoint(endpoint, code string) {
	counter(key("endpoint", endpoint, code)).Add(1)
}

// ObserveEndpoint records a call's duration in the endpoint's histogram.
func ObserveEndpoint(endpoint string, d time.Duration) {
	k := key("endpoint", endpoint)
	var h *histogram
	if v, ok := histograms.Load(k); ok {
		h = v.(*histogram)
	} else {
		v, _ := histograms.LoadOrStore(k, new(histogram))
		h = v.(*histogram)
	}
	s := d.Seconds()
	i := 0
	for ; i < len(bucketBounds); i++ {
		if s <= bucketBounds[i] {
			break
		}
	}
	h.buckets[i].Add(1)
	h.sumUs.Add(uint64(d.Microseconds()))
	h.count.Add(1)
}

// IncAudit counts an emitted audit event by action. token.reuse_detected is
// the highest-signal alert in the system; this is what a pager hangs off.
func IncAudit(action string) {
	counter(key("audit", action)).Add(1)
}

// IncAuditDropped counts an audit event that could NOT be written. The audit
// log is the artefact a regulator reads, so an entry that never lands is not
// a log line to notice later — it is the one number that says the record is
// incomplete. Alert on any increase.
func IncAuditDropped(action string) {
	counter(key("audit_dropped", action)).Add(1)
}

// IncJob counts a maintenance job run by outcome: ok, error, or skipped
// (another replica held the advisory lock — normal, not a failure).
func IncJob(job, result string) {
	counter(key("job", job, result)).Add(1)
}

// SetSnapshotLoaded records when a tenant's gate snapshot was loaded. The
// alert is on staleness: past ANUBIS_SNAPSHOT_MAX_AGE the gate fails closed
// and /readyz pulls the instance from the balancer.
func SetSnapshotLoaded(tenant string, t time.Time) {
	k := key("snapshot", tenant)
	if v, ok := gauges.Load(k); ok {
		v.(*atomic.Int64).Store(t.Unix())
		return
	}
	v, _ := gauges.LoadOrStore(k, new(atomic.Int64))
	v.(*atomic.Int64).Store(t.Unix())
}

// PoolStats is the subset of pgxpool.Stat worth alerting on.
type PoolStats struct {
	Acquired, Idle, Total, Max int64
	EmptyAcquireCount          int64
}

// RegisterPoolStats wires the database pool; read at scrape time.
func RegisterPoolStats(fn func() PoolStats) {
	poolStats.Store(&fn)
}

// SetBuildInfo records the running version once at boot.
func SetBuildInfo(version string) {
	buildInfo.Store(&version)
}
