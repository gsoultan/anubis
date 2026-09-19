package auditrquery

import (
	"time"

	"github.com/gsoultan/storm"
)

// DecisionCountsRow is the allow/deny split over a window.
type DecisionCountsRow struct {
	Allows int64
	Denies int64
}

// CountDecisions24h is the dashboard's decision volume for the last day.
//
// Two counts in one pass with FILTER rather than two queries or a GROUP BY the
// caller has to reassemble — the table is large and this scans it once.
//
// The numbers are FLOORS, not totals: authorize events are sampled under
// pressure, so a quiet period is exact and a busy one undercounts. A dashboard
// that presents them as exact is lying in exactly the conditions that matter.
var CountDecisions24h = storm.SQL[DecisionCountsRow](`
SELECT count(*) FILTER (WHERE result = 'allow') AS allows,
       count(*) FILTER (WHERE result = 'deny')  AS denies
FROM audit_log
WHERE tenant_id = $1 AND action = 'authorize'
  AND occurred_at > now() - interval '24 hours'`)

// ReuseSignalRow is the count and recency of refresh-token reuse.
//
// Latest is a time.Time, not a string. Casting a timestamptz ::text hands back
// PostgreSQL's own rendering — "2026-09-17 14:34:01.109397+00" — which is not
// RFC 3339 and not what time.Parse reads by default, so the boundary crossing
// that looks free costs a layout constant nobody can check by eye.
type ReuseSignalRow struct {
	N      int64
	Latest time.Time
}

// ReuseSignal counts stolen-token events worth a red banner.
//
// COALESCE to epoch so the zero case scans: max() over no rows is NULL, and a
// dashboard asking "when was the last one" for a tenant that has never had one
// is the common case, not an edge.
var ReuseSignal = storm.SQL[ReuseSignalRow](`
SELECT count(*) AS n,
       COALESCE(max(occurred_at), 'epoch'::timestamptz) AS latest
FROM audit_log
WHERE tenant_id = $1 AND action = 'token.reuse_detected'
  AND occurred_at > now() - interval '7 days'`)
