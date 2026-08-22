package repository

import "time"

// Clock is the test seam for time.
type Clock interface {
	Now() time.Time
}
