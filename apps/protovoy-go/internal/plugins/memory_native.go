//go:build !js

package plugins

import (
	"github.com/tetratelabs/wazero/experimental"
	"sync"
)

type memoryBudget struct {
	mu          sync.Mutex
	used, limit uint64
}
type boundedMemory struct {
	budget   *memoryBudget
	bytes    []byte
	reserved uint64
}

func (b *memoryBudget) Allocate(_, max uint64) experimental.LinearMemory {
	return &boundedMemory{budget: b}
}
func (m *boundedMemory) Reallocate(size uint64) []byte {
	b := m.budget
	b.mu.Lock()
	defer b.mu.Unlock()
	if size <= m.reserved {
		return m.bytes[:int(size)]
	}
	growth := size - m.reserved
	if growth > b.limit-b.used {
		return nil
	}
	next := make([]byte, int(size))
	copy(next, m.bytes)
	b.used += growth
	m.reserved = size
	m.bytes = next
	return next
}
func (m *boundedMemory) Free() {
	b := m.budget
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used -= m.reserved
	m.reserved = 0
	m.bytes = nil
}
