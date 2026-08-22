package ratelimit

import "sync"

type shard struct {
	mu sync.Mutex
	m  map[string]*bucket
}
