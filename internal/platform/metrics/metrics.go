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

// IncRefreshReuse counts detected refresh-token theft, split by whether the
// automatic containment actually landed.
//
// The split is the point. Detection already pages through the audit log, and
// the revocations behind it used to have their errors discarded — so a page
// saying a stolen token was found could arrive while the token family stayed
// usable, and nothing said so. contained="false" means act NOW: the
// revocation has to be done by hand.
func IncRefreshReuse(contained bool) {
	result := "contained"
	if !contained {
		result = "not_contained"
	}
	counter(key("refresh_reuse", result)).Add(1)
}

// IncJob counts a maintenance job run by outcome: ok, error, or skipped.
//
// Exposed as the label `maintenance_job`, not `job`. Prometheus reserves `job`
// and `instance` for the scrape target: an exporter that emits its own `job`
// gets it silently renamed to `exported_job`, so every query and annotation
// written against `job` matches the SCRAPE job name instead — which is to say
// the alert that exists to name the failing job names the wrong thing, and
// looks fine doing it.
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

// IncSnapshotRefresh counts gate snapshot refreshes by outcome:
//
//	rebuilt    the catalog version moved, so the snapshot was reloaded
//	unchanged  the version matched, so the ~92 MB rebuild was skipped
//	verify     a periodic rebuild done regardless of the version
//	failed     the load errored; the previous snapshot is still being served
//
// Without this an operator cannot tell a working version gate from one that
// silently rebuilds every poll — or, worse, one that skips forever because an
// invalidation trigger went missing. Expect mostly "unchanged", a "verify"
// per tenant per max-age window, and "rebuilt" to track real catalog edits.
func IncSnapshotRefresh(tenant, result string) {
	counter(key("snaprefresh", tenant, result)).Add(1)
}

// IncScopeSync counts SCHEDULED structure syncs by outcome: ok or failed.
//
// The maintenance-job counter cannot see this. RunDue deliberately returns nil
// when an individual source fails, so that one bad feed does not stop every
// other tenant's — which means anubis_job_runs_total{job="scope_sync"} stays
// "ok" while every source in the installation is failing. This is the counter
// that knows the difference.
//
// Labelled by axis, not by source id: an axis is registered by an operator and
// bounded by the axis registry, while source ids grow with tenants. The log
// line carries source and tenant for the follow-up.
//
// Manual runs are not counted. Somebody pressed the button and watched the
// result; the operator this is for is the one who is not looking.
func IncScopeSync(axis, result string) {
	counter(key("scopesync", axis, result)).Add(1)
}

// SetSnapshotNodes records how many scope nodes a tenant's snapshot holds.
// Every instance holds every tenant's snapshot, so this is the number to size
// memory against: roughly 95 bytes per node (ADR-0015).
func SetSnapshotNodes(tenant string, n int) {
	k := key("snapnodes", tenant)
	if v, ok := gauges.Load(k); ok {
		v.(*atomic.Int64).Store(int64(n))
		return
	}
	v, _ := gauges.LoadOrStore(k, new(atomic.Int64))
	v.(*atomic.Int64).Store(int64(n))
}

// IncDeprecated counts a call to an RPC that is on its way out.
//
// This is not an alert, it is the number you need to decide whether removal
// is safe. A deprecation comment tells a reader; it does not tell you whether
// anything in your estate still calls the thing at three in the morning. Zero
// for a release cycle is the evidence; anything else names the caller's
// endpoint so you can go and find it.
func IncDeprecated(rpc string) {
	counter(key("deprecated", rpc)).Add(1)
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
