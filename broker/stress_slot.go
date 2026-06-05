package broker

import (
	"sync"
	"sync/atomic"
)

type stressSlot struct {
	mu     sync.Mutex
	stress atomic.Pointer[SymbolStress]
}

func (cache *StressCache) slotFor(symbol string) *stressSlot {
	slot, _ := cache.slots.LoadOrStore(symbol, &stressSlot{})

	return slot.(*stressSlot)
}

func (slot *stressSlot) value() (SymbolStress, bool) {
	current := slot.stress.Load()

	if current == nil {
		return SymbolStress{}, false
	}

	return *current, true
}

func (slot *stressSlot) store(stress SymbolStress) {
	next := stress
	slot.stress.Store(&next)
}
