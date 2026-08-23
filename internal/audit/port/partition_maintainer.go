package auditport

import "context"

// PartitionMaintainer provisions the range partitions audit_log and
// refresh_tokens need ahead of time. Retrofitting partitioning onto a large
// hot table requires a full rewrite under ACCESS EXCLUSIVE — on an auth
// service that is downtime for every application at once (ADR-0005 §6).
type PartitionMaintainer interface {
	EnsurePartitions(ctx context.Context) error
}
