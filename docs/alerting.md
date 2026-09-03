# Alerting

Every rule pairs a metric with the runbook section that answers it. Metrics
are served in Prometheus text format at `/metrics` on the **debug listener**
(`ANUBIS_DEBUG_LISTEN`) — they describe the installation's operation and stay
off the public API surface. In Kubernetes, bind the debug listener to the pod
IP and restrict it with a NetworkPolicy; the scraper is the only client.

## Page immediately

| Alert | Rule (PromQL) | Why, and what to do |
| :--- | :--- | :--- |
| **Audit log incomplete** | `increase(anubis_audit_dropped_total{action!="authorize"}[15m]) > 0` | An audit event could not be written. The log a regulator reads now has a hole in it, and the only other record is a log line. The `action` label says which call it was; the server's error log says why. Treat as an incident, not a warning — an audit trail that silently loses entries is worse than one that is visibly broken. |
| **Authorize decisions outrunning the audit writer** | `increase(anubis_audit_dropped_total{action="authorize"}[15m]) > 0` | Capacity, not correctness: `authorize` is the one action allowed to drop rather than block, because back-pressure there would turn a slow database into a slow `authorize()`. Sustained drops mean the audit writer cannot keep up with decision volume — check database write latency before assuming the rate is the problem. |
| **Refresh token stolen** | `increase(anubis_audit_events_total{action="token.reuse_detected"}[5m]) > 0` | The highest-signal event in the system: a consumed refresh token was replayed. Anubis already revoked the family and session. Run [operations.md — Incident: refresh token reuse](operations.md#incident-refresh-token-reuse). |
| **Gate snapshot stale** | `time() - anubis_gate_snapshot_loaded_timestamp_seconds > 300` (match `ANUBIS_SNAPSHOT_MAX_AGE`) | Past max age the gate **fails closed**: the instance is denying traffic it should allow. `/readyz` also fails, so the balancer should already be pulling it — this alert is the "why did readiness fail" answer. Check database connectivity and the `snapshot load failed` log line. |
| **Internal error rate** | `sum(rate(anubis_endpoint_requests_total{code="internal"}[5m])) > 0.1` | Internal means a bug or a dependency down, never a caller mistake. Correlate with `request_id` in logs. |
| **Maintenance job failing** | `increase(anubis_job_runs_total{result="error"}[2h]) > 1` | `partitions` failing means rows land in the DEFAULT partition; `retention` failing means PII outlives its deadline; `signing_key_expiry` failing means nobody is watching key age. Job name is in the label; each has a section in [operations.md](operations.md#maintenance-jobs). |

## Warn

| Alert | Rule | Why |
| :--- | :--- | :--- |
| **Credential-guessing pressure** | `sum(rate(anubis_endpoint_requests_total{code="rate_limited"}[15m])) by (endpoint) > 1` | The limiter is doing its job; sustained pressure on `auth.login` or `platform.login` is a campaign, not noise. Platform accounts are few and named — check WHICH account in the logs. |
| **Login latency shift** | `histogram_quantile(0.95, rate(anubis_endpoint_duration_seconds_bucket{endpoint="auth.login"}[15m])) > 0.2` | Login is KDF-dominated (~50 ms) by design. A p95 past 200 ms means pool saturation or database pressure — not something to "fix" by weakening the KDF. |
| **Authorize latency budget** | `histogram_quantile(0.95, rate(anubis_endpoint_duration_seconds_bucket{endpoint="authz.authorize"}[15m])) > 0.002` | The enforced budget is p95 < 2 ms. A breach here degrades every consuming application at once. |
| **Pool saturation** | `rate(anubis_db_pool_empty_acquires_total[5m]) > 0` | Callers are waiting for connections. Check `anubis_db_pool_*` gauges against `ANUBIS_DB_MAX_CONNS`; size to the database, not the app. |
| **Version skew** | `count(count by (version) (anubis_build_info)) > 1` for > 1h | A rollout that never finished: two versions serving side by side long after a deploy window should have closed. |
| **Snapshot never rebuilds** | `rate(anubis_gate_snapshot_refresh_total{result=~"rebuilt\|verify"}[30m]) == 0` while `result="unchanged"` is climbing | The gate skips rebuilding a snapshot whose catalog version has not moved ([ADR-0015](adr/0015-scope-hierarchy-at-scale.md)). If a tenant is *never* rebuilt, either it is genuinely idle, or an invalidation trigger was lost and the version stopped moving — which looks identical from the outside and means stale authorization. The periodic `verify` rebuild bounds this to one max-age window, so its absence is the signal. |
| **Snapshot load failing** | `rate(anubis_gate_snapshot_refresh_total{result="failed"}[15m]) > 0` | The instance is serving the previous snapshot (fail-static) and will fail closed once it passes max age. This fires *before* the staleness page and names the tenant. |

## Deliberately not alerted

- `anubis_job_runs_total{result="skipped"}` — another replica held the
  advisory lock. That is the coordination mechanism working, not a failure.
- `anubis_gate_snapshot_refresh_total{result="unchanged"}` climbing — the
  version gate skipping a rebuild it did not need. That is the optimisation
  working; it should be the overwhelming majority of refreshes.
- `anubis_deprecated_rpc_total{rpc}` — not an alert, a **removal signal**. A
  deprecation comment tells whoever reads the schema; it does not tell you
  whether something in the estate still calls the RPC at three in the morning.
  Zero across a release cycle is the evidence that removing it is safe.
  Currently counts `GetSigninPage` and `PutSigninPage`, which write to a table
  no hosted page has rendered from since migration 0024.
- `anubis_gate_snapshot_scope_nodes` — not an alert, a **capacity gauge**.
  Every instance holds every tenant's snapshot at roughly 95 bytes per scope
  node, so `sum(anubis_gate_snapshot_scope_nodes) * 95` is the floor on gate
  memory. Watch it grow; do not page on it.
- `code="permission_denied"` / `code="invalid_argument"` rates — caller
  mistakes are the caller's dashboard, not the operator's pager. They become
  interesting only as a sudden delta, which the internal-error and
  rate-limit rules already frame.
- Per-tenant sign-in volume — product analytics, not operations. The audit
  log is the source of truth for that.
