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

// InstallationTenant is the tenant id under which the platform plane records
// its own events.
//
// audit_log.tenant_id is NOT NULL and every index leads with it, but a
// platform user belongs to no tenant (ADR-0011) — they operate the
// installation. Events like a failed platform login therefore had no tenant
// to be filed under, and the auditor dropped them at the door: for as long as
// that was true, no failed administrator login was recorded anywhere, and
// token.reuse_detected — the highest-signal alert in the system — could not
// fire for a platform user at all.
//
// The nil UUID is the installation. uuidv7 never produces all-zeros, so it
// cannot collide with a real tenant, and because the hash chain is per tenant
// the installation gets a chain of its own — which is what a separate
// population should have.
const InstallationTenant = "00000000-0000-0000-0000-000000000000"

// TenantOrInstallation files an event under its tenant, or under the
// installation when it has none.
func TenantOrInstallation(tenantID string) string {
	if tenantID == "" {
		return InstallationTenant
	}
	return tenantID
}
