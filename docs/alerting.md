# Alerting

Every rule pairs a metric with the runbook section that answers it. Metrics
are served in Prometheus text format at `/metrics` on the **debug listener**
(`ANUBIS_DEBUG_LISTEN`) — they describe the installation's operation and stay
off the public API surface. In Kubernetes, bind the debug listener to the pod
IP and restrict it with a NetworkPolicy; the scraper is the only client.

## Page immediately

| Alert | Rule (PromQL) | Why, and what to do |
| :--- | :--- | :--- |
| **Audit log incomplete** | `increase(anubis_audit_dropped_total[15m]) > 0` | An audit event could not be written. The log a regulator reads now has a hole in it, and the only other record is a log line. The `action` label says which call it was; the server's error log says why. Treat as an incident, not a warning — an audit trail that silently loses entries is worse than one that is visibly broken. |
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

## Deliberately not alerted

- `anubis_job_runs_total{result="skipped"}` — another replica held the
  advisory lock. That is the coordination mechanism working, not a failure.
- `code="permission_denied"` / `code="invalid_argument"` rates — caller
  mistakes are the caller's dashboard, not the operator's pager. They become
  interesting only as a sudden delta, which the internal-error and
  rate-limit rules already frame.
- Per-tenant sign-in volume — product analytics, not operations. The audit
  log is the source of truth for that.
