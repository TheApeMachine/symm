package resonance

import (
	"sync"
	"sync/atomic"

	"github.com/theapemachine/nomagique/learning"
)

type batchEntry struct {
	slot   int
	symbol string
	input  []float64
}

type settleOutcome struct {
	symbol     string
	input      []float64
	latent     []float64
	surprise   float64
	energy     float64
	wireSource *learning.ResonanceManifold
}

type batchEngine interface {
	Close()
	Capacity() int
	Settle(entries []batchEntry) ([]settleOutcome, error)
}

type slotRegistry struct {
	capacity     int
	slotBySymbol sync.Map
	assigned     atomic.Int32
}

func newSlotRegistry(capacity int) *slotRegistry {
	if capacity <= 0 {
		capacity = 1
	}

	return &slotRegistry{
		capacity: capacity,
	}
}

func (registry *slotRegistry) assign(symbol string) (int, bool) {
	if raw, ok := registry.slotBySymbol.Load(symbol); ok {
		return raw.(int), true
	}

	slot := registry.assigned.Add(1) - 1

	if int(slot) >= registry.capacity {
		return 0, false
	}

	actual, loaded := registry.slotBySymbol.LoadOrStore(symbol, int(slot))

	if loaded {
		return actual.(int), true
	}

	return int(slot), true
}
