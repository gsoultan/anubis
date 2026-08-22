package ratelimit

import "time"

type bucket struct {
	tokens float64
	last   time.Time
}
