package share

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// MaxLife bounds how long a share may last. Ninety days is the same fuse a
// platform key and an imported password get: long enough for any piece of
// work, short enough that "temporary" cannot quietly mean forever.
const MaxLife = 90 * 24 * time.Hour

// MaxPermissions bounds one share. A share that names forty permissions is
// a role, and a role is granted, not shared.
const MaxPermissions = 32

var kindRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)

var (
	ErrBadKind = errors.New("share: record kind must be a short lowercase code")
	ErrBadRef  = errors.New("share: record ref is required and must fit on one line")
)

// RecordKey is how a record is named everywhere: in the row, in the
// "_record" target, in the audit log. kind is the application's noun
// (job_requisition); ref is the application's own identifier for the one
// record. The two are joined with "/", and ref may itself contain "/".
func RecordKey(kind, ref string) (string, error) {
	kind = strings.TrimSpace(kind)
	ref = strings.TrimSpace(ref)
	if !kindRe.MatchString(kind) {
		return "", ErrBadKind
	}
	if ref == "" || len(ref) > 200 || strings.ContainsAny(ref, "\n\r\t") {
		return "", ErrBadRef
	}
	return kind + "/" + ref, nil
}

// SplitRecordKey is RecordKey's inverse.
func SplitRecordKey(key string) (kind, ref string) {
	kind, ref, _ = strings.Cut(key, "/")
	return kind, ref
}
