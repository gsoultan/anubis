package apiconnect

import (
	"testing"
	"time"
)

// A key's "last used" is written at most once a minute, not on every request:
// the write locked the key's row, and every caller sharing that key queued
// behind it.
func TestUsageIsRecordedOncePerMinute(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	recent := now.Add(-30 * time.Second)
	old := now.Add(-time.Minute)
	for name, c := range map[string]struct {
		last *time.Time
		want bool
	}{
		"never used":        {nil, true},
		"used 30s ago":      {&recent, false},
		"used a minute ago": {&old, true},
	} {
		if got := usageStale(c.last, now); got != c.want {
			t.Errorf("%s: write=%v want %v", name, got, c.want)
		}
	}
}
