package relation

import (
	"fmt"
	"iter"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ObservationStore is resident streaming state: one bounded chronological ring
per coordinate key. Capacity is infrastructure provenance, not a statistical
claim; eviction is always the oldest observation, never value-based.

Each ring is written twice (slot and slot+capacity), so the retained history
is always one contiguous chronological window that is read in place and never
copied or re-sorted.

A non-nil run writes: every arrival is *map[string][]float64, coordinate key
to a flattened batch {at0, raw0, at1, raw1, ...}. An empty batch registers
the coordinate without observing it. The arrival is yielded back once it has
been retained.

A nil run reads: it yields one *map[string][]float64 of every registered
coordinate to its flattened chronological window. The windows are valid only
for the duration of the yield (the store holds its read lock across it) and
must not be mutated.
*/
type ObservationStore struct {
	*core.PrimitiveError
	capacity int
	gate     sync.RWMutex
	entries  map[string][]float64
	heads    map[string]int
	sizes    map[string]int
	windows  map[string][]float64
}

/*
NewObservationStore builds a store with the given per-coordinate capacity. A
non-positive capacity is a domain failure and every run yields nothing.
*/
func NewObservationStore(capacity int) *ObservationStore {
	op := &ObservationStore{
		PrimitiveError: core.NewPrimitiveError(),
		capacity:       capacity,
		entries:        make(map[string][]float64),
		heads:          make(map[string]int),
		sizes:          make(map[string]int),
		windows:        make(map[string][]float64),
	}

	if capacity < 1 {
		op.Error(fmt.Errorf(
			"%w: relation: store capacity %d must be positive",
			core.ErrDomain, capacity,
		))
	}

	return op
}

func (op *ObservationStore) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		if in == nil {
			op.gate.RLock()
			defer op.gate.RUnlock()
			yield(unsafe.Pointer(&op.windows))
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			batch := *(*map[string][]float64)(arriving)

			for key, observations := range batch {
				if len(observations)%2 != 0 {
					op.Error(fmt.Errorf(
						"%w: relation: batch for %q is not {at, raw} pairs",
						core.ErrShape, key,
					))
					return
				}
			}

			op.gate.Lock()

			for key, observations := range batch {
				ring, held := op.entries[key]

				if !held {
					ring = make([]float64, 4*op.capacity)
					op.entries[key] = ring
					op.windows[key] = ring[:0]
				}

				head, size := op.heads[key], op.sizes[key]

				for index := 0; index+1 < len(observations); index += 2 {
					ring[2*head] = observations[index]
					ring[2*head+1] = observations[index+1]
					ring[2*(head+op.capacity)] = observations[index]
					ring[2*(head+op.capacity)+1] = observations[index+1]
					head = (head + 1) % op.capacity

					if size < op.capacity {
						size++
					}
				}

				start := (head - size + op.capacity) % op.capacity
				op.heads[key], op.sizes[key] = head, size
				op.windows[key] = ring[2*start : 2*(start+size)]
			}

			op.gate.Unlock()

			if !yield(arriving) {
				return
			}
		}
	}
}
