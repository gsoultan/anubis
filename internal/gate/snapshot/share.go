package snapshot

import "time"

// Share is one live record share as the gate carries it: what the recipient
// may do on one record, until when. Mirrors the EXISTS branch 0058 added to
// authorize(): places do not apply, every identity and permission gate does.
type Share struct {
	Record      string
	Permissions map[string]bool
	ValidUntil  time.Time
}

// sharedRecordAllows is the share half of Evaluate: the identity and
// permission gates have already passed, the grants said no, and the caller
// named a record.
func (d *Data) sharedRecordAllows(identityID, permission, record string, now time.Time) bool {
	for _, s := range d.SharesByIdentity[identityID] {
		if s.Record == record && s.ValidUntil.After(now) && s.Permissions[permission] {
			return true
		}
	}
	return false
}
