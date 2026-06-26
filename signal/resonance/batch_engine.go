package resonance

import (
	"sync"

	"github.com/theapemachine/nomagique/learning"
)

type batchEntry struct {
	slot   int
	symbol string
	input  []float64
}

type wireSnapshotSource interface {
	WireSnapshot() ([]learning.ResonanceLayerWire, float64, float64)
}

type settleOutcome struct {
	symbol     string
	input      []float64
	latent     []float64
	surprise   float64
	energy     float64
	wireSource wireSnapshotSource
}

type batchEngine interface {
	Close()
	Capacity() int
	Settle(entries []batchEntry) ([]settleOutcome, error)
	// Reset clears learned state for the given slots so a slot reused by a new
	// symbol does not inherit the prior symbol's converged weights (which would
	// understate the newcomer's reconstruction surprise).
	Reset(slots []int) error
}

type slotRegistry struct {
	mutex        sync.Mutex
	capacity     int
	slotBySymbol map[string]int
	symbolBySlot map[int]string
	freeSlots    []int
	nextSlot     int
}

func newSlotRegistry(capacity int) *slotRegistry {
	if capacity <= 0 {
		capacity = 1
	}

	return &slotRegistry{
		capacity:     capacity,
		slotBySymbol: make(map[string]int, capacity),
		symbolBySlot: make(map[int]string, capacity),
	}
}

func (registry *slotRegistry) assign(symbol string) (int, bool) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	if slot, ok := registry.slotBySymbol[symbol]; ok {
		return slot, true
	}

	slot, ok := registry.takeSlot()

	if !ok {
		return 0, false
	}

	registry.slotBySymbol[symbol] = slot
	registry.symbolBySlot[slot] = symbol

	return slot, true
}

/*
takeSlot returns a free slot — a reclaimed one first, then a fresh index up to
capacity. Caller holds the mutex.
*/
func (registry *slotRegistry) takeSlot() (int, bool) {
	if last := len(registry.freeSlots) - 1; last >= 0 {
		slot := registry.freeSlots[last]
		registry.freeSlots = registry.freeSlots[:last]

		return slot, true
	}

	if registry.nextSlot >= registry.capacity {
		return 0, false
	}

	slot := registry.nextSlot
	registry.nextSlot++

	return slot, true
}

/*
retain frees the slots of every symbol absent from live so a rotating universe
reuses slots instead of leaking them, returning the freed slots so the caller
can clear their learned state before reuse.
*/
func (registry *slotRegistry) retain(live []string) []int {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	keep := make(map[string]struct{}, len(live))

	for _, symbol := range live {
		keep[symbol] = struct{}{}
	}

	freed := make([]int, 0)

	for symbol, slot := range registry.slotBySymbol {
		if _, ok := keep[symbol]; ok {
			continue
		}

		delete(registry.slotBySymbol, symbol)
		delete(registry.symbolBySlot, slot)
		registry.freeSlots = append(registry.freeSlots, slot)
		freed = append(freed, slot)
	}

	return freed
}
