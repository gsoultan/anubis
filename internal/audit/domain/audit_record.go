package auditdomain

import "time"

type AuditRecord struct {
	ID         string
	OccurredAt time.Time
	Seq        int64
	ActorID    string
	ActorKind  string
	TargetID   string
	SessionID  string
	Action     string
	Result     string
	IP         string
	Detail     []byte
	PrevHash   []byte
	EntryHash  []byte
}
