package metrics

import (
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// A rule naming a metric the exporter does not emit never fires, and evaluates
// to "ok" with an empty result — indistinguishable from a condition that is
// simply false. The place you find out is the incident it was written for.
// packaging/anubis.rules.yml was checked against a live scrape when it was
// written (its header says so); this keeps that true. Every anubis_ metric a
// rule EXPRESSION references must appear in a real /metrics render.
//
// Scoped to expr, not the whole file: a description says "check the
// anubis_db_pool_* gauges", and a glob is not a metric name.
func TestEveryAlertRuleMetricIsExported(t *testing.T) {
	const rulesPath = "../../../packaging/anubis.rules.yml"
	raw, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read %s: %v", rulesPath, err)
	}
	names := metricsInExprs(string(raw))
	if len(names) == 0 {
		t.Fatal("no anubis_ metrics found in any rule expression — the extraction is broken, not the rules")
	}

	// Touch every family so histogram buckets and each counter are present;
	// the family headers are emitted unconditionally, so a name is present
	// without touching its counter — and touching a counter the sibling
	// exposition test asserts an exact value on would break it, since these
	// metrics are package globals shared across the run. Only two names need
	// producing: a histogram bucket needs one observation (on a throwaway
	// endpoint, so no shared counter moves), and build_info and the pool gauge
	// need their setter. Both setters replace rather than accumulate, so order
	// under -shuffle does not matter.
	ObserveEndpoint("rules_probe_endpoint", 3*time.Millisecond)
	SetBuildInfo("rules-probe")
	RegisterPoolStats(func() PoolStats { return PoolStats{EmptyAcquireCount: 1} })

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()

	for _, name := range names {
		if !strings.Contains(body, name) {
			t.Errorf("alert rule expression references %q, which /metrics never exposes — "+
				"the rule can never fire", name)
		}
	}
}

var (
	exprLine   = regexp.MustCompile(`(?m)^(\s*)expr:\s*(.*)$`)
	metricName = regexp.MustCompile(`anubis_[a-z][a-z0-9_]*`)
)

// metricsInExprs returns every anubis_ metric name that appears in an alert's
// expr, inline or in a block scalar (expr: |). Block-scalar lines are the ones
// indented past the expr: key.
func metricsInExprs(rules string) []string {
	lines := strings.Split(rules, "\n")
	var exprText strings.Builder
	for i := 0; i < len(lines); i++ {
		m := exprLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent, rest := m[1], strings.TrimSpace(m[2])
		if rest != "" && rest != "|" && rest != ">" && rest != "|-" && rest != ">-" {
			exprText.WriteString(rest)
			exprText.WriteByte('\n')
			continue
		}
		// Block scalar: consume the more-indented lines that follow.
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				continue
			}
			lead := len(lines[j]) - len(strings.TrimLeft(lines[j], " "))
			if lead <= len(indent) {
				break
			}
			exprText.WriteString(lines[j])
			exprText.WriteByte('\n')
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range metricName.FindAllString(exprText.String(), -1) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
