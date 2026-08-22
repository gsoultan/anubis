package ratelimit

// Limit describes a bucket class.
type Limit struct {
	PerMinute float64 // sustained refill rate
	Burst     float64 // bucket capacity
}
