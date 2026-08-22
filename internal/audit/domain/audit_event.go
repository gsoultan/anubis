// Package repository defines the interfaces the usecases program against and the
// records that cross them. Conventions: one type per file; no interface wider
// than 15 methods — wider capabilities compose by embedding (catalog_store.go,
// store.go). Usecases depend on THIS package; internal/adapter/postgres
// implements it. Neither proto nor pgx types appear here.
package auditdomain

// AuditEvent is one hash-chained audit record. Detail is snapshotted decision
// input, already JSON.
type AuditEvent struct {
	TenantID  string
	ActorID   string // "" = system
	ActorKind string // identity | service | system
	TargetID  string
	SessionID string
	Action    string
	Result    string // allow | deny | error
	IP        string
	Detail    []byte
}
