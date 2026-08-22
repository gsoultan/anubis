package keyring

import "sync/atomic"

// Manager provides lock-free reads with atomic ring swaps on reload.
type Manager struct {
	ring atomic.Pointer[Ring]
}

func NewManager(r *Ring) *Manager {
	m := &Manager{}
	m.ring.Store(r)
	return m
}

func (m *Manager) Ring() *Ring  { return m.ring.Load() }
func (m *Manager) Swap(r *Ring) { m.ring.Store(r) }
