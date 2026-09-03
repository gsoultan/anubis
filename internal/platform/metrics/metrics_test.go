package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// One scrape must carry every family with correct Prometheus text framing —
// a malformed exposition fails silently: the scraper just records nothing.
func TestExpositionCarriesAllFamilies(t *testing.T) {
	IncEndpoint("auth.login", "ok")
	IncEndpoint("auth.login", "invalid_credentials")
	ObserveEndpoint("auth.login", 48*time.Millisecond)
	IncAudit("token.reuse_detected")
	IncJob("retention", "ok")
	IncJob("retention", "skipped")
	SetSnapshotLoaded("impack", time.Unix(1_700_000_000, 0))
	SetBuildInfo("test-sha")
	IncDeprecated("TenantAdminService/PutSigninPage")
	RegisterPoolStats(func() PoolStats {
		return PoolStats{Acquired: 1, Idle: 2, Total: 3, Max: 4, EmptyAcquireCount: 5}
	})

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()

	for _, want := range []string{
		`anubis_endpoint_requests_total{endpoint="auth.login",code="ok"} 1`,
		`anubis_endpoint_requests_total{endpoint="auth.login",code="invalid_credentials"} 1`,
		`anubis_endpoint_duration_seconds_count{endpoint="auth.login"} 1`,
		// 48 ms falls in the le=0.05 bucket and every one after it.
		`anubis_endpoint_duration_seconds_bucket{endpoint="auth.login",le="0.05"} 1`,
		`anubis_endpoint_duration_seconds_bucket{endpoint="auth.login",le="0.025"} 0`,
		`anubis_endpoint_duration_seconds_bucket{endpoint="auth.login",le="+Inf"} 1`,
		`anubis_audit_events_total{action="token.reuse_detected"} 1`,
		`anubis_job_runs_total{job="retention",result="ok"} 1`,
		`anubis_job_runs_total{job="retention",result="skipped"} 1`,
		`anubis_gate_snapshot_loaded_timestamp_seconds{tenant="impack"} 1700000000`,
		`anubis_deprecated_rpc_total{rpc="TenantAdminService/PutSigninPage"} 1`,
		`anubis_build_info{version="test-sha"} 1`,
		`anubis_db_pool_empty_acquires_total 5`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q\n---\n%s", want, body)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("wrong content type: %q", ct)
	}
}
