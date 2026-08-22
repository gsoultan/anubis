package auditdomain

import "time"

type AuditQuery struct {
	ActorID   string
	Action    string
	From      *time.Time
	To        *time.Time
	BeforeSeq *int64
	Limit     int
}
