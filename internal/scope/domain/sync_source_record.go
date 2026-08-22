package scopedomain

import "time"

type SyncSourceRecord struct {
	ID        string
	Axis      string
	Kind      string
	Status    string
	Config    []byte
	LastRunAt *time.Time
}
