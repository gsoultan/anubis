package auditrquery

import "github.com/gsoultan/storm"

// EnsureAuditPartitions creates the coming months' audit partitions.
//
// The partitions are made HERE, by a database function on a schedule, and not
// by the model: storm knows audit_log is partitioned and deliberately neither
// creates nor drops its partitions, so a migration diff never proposes
// deleting a month of entries. See auditrmodel.AuditLog.
var EnsureAuditPartitions = storm.SQL[DoneRow](`
SELECT (ensure_month_partitions('audit_log', 'occurred_at', 3) IS NULL) AS done`)

// EnsureRefreshPartitions does the same for refresh_tokens, which is
// partitioned on expires_at so an expired month can be dropped whole rather
// than deleted row by row.
var EnsureRefreshPartitions = storm.SQL[DoneRow](`
SELECT (ensure_month_partitions('refresh_tokens', 'expires_at', 3) IS NULL) AS done`)
