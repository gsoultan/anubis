package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Handler serves the registry in Prometheus text exposition format v0.0.4.
// Mounted on the debug listener beside pprof/expvar: metrics describe the
// installation's operation and stay off the public surface.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		var b strings.Builder
		writeEndpoints(&b)
		writeCounterFamily(&b, "anubis_audit_events_total",
			"Audit events emitted, by action. token.reuse_detected means a refresh token was stolen.",
			"audit", "action")
		writeCounterFamily(&b, "anubis_audit_dropped_total",
			"Audit events that could not be written. Any increase means the audit log is incomplete.",
			"audit_dropped", "action")
		writeJobFamily(&b)
		writeSnapshots(&b)
		writePool(&b)
		if v := buildInfo.Load(); v != nil {
			fmt.Fprintf(&b, "# TYPE anubis_build_info gauge\nanubis_build_info{version=%q} 1\n", *v)
		}
		_, _ = w.Write([]byte(b.String()))
	})
}

// snapshotKeys returns the registry keys for one family prefix, sorted so
// scrapes are stable and diffs between them mean something.
func familyKeys(m interface{ Range(func(k, v any) bool) }, prefix string) []string {
	var keys []string
	m.Range(func(k, _ any) bool {
		if s := k.(string); strings.HasPrefix(s, prefix+"\xff") {
			keys = append(keys, s)
		}
		return true
	})
	sort.Strings(keys)
	return keys
}

func labelOf(k string, i int) string {
	parts := strings.Split(k, "\xff")
	if i+1 < len(parts) {
		return parts[i+1]
	}
	return ""
}

func writeEndpoints(b *strings.Builder) {
	fmt.Fprint(b, "# HELP anubis_endpoint_requests_total RPCs by endpoint and outcome code.\n")
	fmt.Fprint(b, "# TYPE anubis_endpoint_requests_total counter\n")
	for _, k := range familyKeys(&counters, "endpoint") {
		v, _ := counters.Load(k)
		fmt.Fprintf(b, "anubis_endpoint_requests_total{endpoint=%q,code=%q} %d\n",
			labelOf(k, 0), labelOf(k, 1), v.(interface{ Load() uint64 }).Load())
	}

	fmt.Fprint(b, "# HELP anubis_endpoint_duration_seconds RPC latency by endpoint.\n")
	fmt.Fprint(b, "# TYPE anubis_endpoint_duration_seconds histogram\n")
	for _, k := range familyKeys(&histograms, "endpoint") {
		v, _ := histograms.Load(k)
		h := v.(*histogram)
		name := labelOf(k, 0)
		var cum uint64
		for i, bound := range bucketBounds {
			cum += h.buckets[i].Load()
			fmt.Fprintf(b, "anubis_endpoint_duration_seconds_bucket{endpoint=%q,le=%q} %d\n",
				name, strconv.FormatFloat(bound, 'g', -1, 64), cum)
		}
		cum += h.buckets[len(bucketBounds)].Load()
		fmt.Fprintf(b, "anubis_endpoint_duration_seconds_bucket{endpoint=%q,le=\"+Inf\"} %d\n", name, cum)
		fmt.Fprintf(b, "anubis_endpoint_duration_seconds_sum{endpoint=%q} %g\n",
			name, float64(h.sumUs.Load())/1e6)
		fmt.Fprintf(b, "anubis_endpoint_duration_seconds_count{endpoint=%q} %d\n",
			name, h.count.Load())
	}
}

func writeCounterFamily(b *strings.Builder, family, help, prefix, label string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s counter\n", family, help, family)
	for _, k := range familyKeys(&counters, prefix) {
		v, _ := counters.Load(k)
		fmt.Fprintf(b, "%s{%s=%q} %d\n", family, label,
			labelOf(k, 0), v.(interface{ Load() uint64 }).Load())
	}
}

func writeJobFamily(b *strings.Builder) {
	fmt.Fprint(b, "# HELP anubis_job_runs_total Maintenance job runs by outcome (skipped = another replica held the lock).\n")
	fmt.Fprint(b, "# TYPE anubis_job_runs_total counter\n")
	for _, k := range familyKeys(&counters, "job") {
		v, _ := counters.Load(k)
		fmt.Fprintf(b, "anubis_job_runs_total{job=%q,result=%q} %d\n",
			labelOf(k, 0), labelOf(k, 1), v.(interface{ Load() uint64 }).Load())
	}
}

func writeSnapshots(b *strings.Builder) {
	fmt.Fprint(b, "# HELP anubis_gate_snapshot_loaded_timestamp_seconds When each tenant's gate snapshot was loaded. Alert on age: past max-age the gate fails closed.\n")
	fmt.Fprint(b, "# TYPE anubis_gate_snapshot_loaded_timestamp_seconds gauge\n")
	for _, k := range familyKeys(&gauges, "snapshot") {
		v, _ := gauges.Load(k)
		fmt.Fprintf(b, "anubis_gate_snapshot_loaded_timestamp_seconds{tenant=%q} %d\n",
			labelOf(k, 0), v.(interface{ Load() int64 }).Load())
	}
}

func writePool(b *strings.Builder) {
	fn := poolStats.Load()
	if fn == nil {
		return
	}
	s := (*fn)()
	fmt.Fprint(b, "# TYPE anubis_db_pool_acquired_conns gauge\n")
	fmt.Fprintf(b, "anubis_db_pool_acquired_conns %d\n", s.Acquired)
	fmt.Fprint(b, "# TYPE anubis_db_pool_idle_conns gauge\n")
	fmt.Fprintf(b, "anubis_db_pool_idle_conns %d\n", s.Idle)
	fmt.Fprint(b, "# TYPE anubis_db_pool_total_conns gauge\n")
	fmt.Fprintf(b, "anubis_db_pool_total_conns %d\n", s.Total)
	fmt.Fprint(b, "# TYPE anubis_db_pool_max_conns gauge\n")
	fmt.Fprintf(b, "anubis_db_pool_max_conns %d\n", s.Max)
	fmt.Fprint(b, "# HELP anubis_db_pool_empty_acquires_total Acquires that had to wait for a free connection — saturation signal.\n")
	fmt.Fprint(b, "# TYPE anubis_db_pool_empty_acquires_total counter\n")
	fmt.Fprintf(b, "anubis_db_pool_empty_acquires_total %d\n", s.EmptyAcquireCount)
}
