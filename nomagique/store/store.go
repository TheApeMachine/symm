package store

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
StoreType defines the internal buffer storage implementation strategy.
*/
type StoreType int

const (
	DynamicRing StoreType = iota
)

// MinimumStoreCapacity is the minimum sample count required for dispersion statistics (n - 1 >= 1).
const MinimumStoreCapacity = 2

/*
Store is a stateful primitive holding a dynamically sized collection of samples.
When used as a passive sidecar buffer (Sink) without a Reduce slot, Step returns 0
so it contributes nothing to a parallel summation (Law of Sinks).
When configured with a Reduce slot, Step returns the reduction over the emergent window.
*/
type Store struct {
	Type     StoreType
	Adaptive adaptive.Window
	Reduce   types.Reduction

	Buffer []types.Number
}

// Step records the sample into memory and applies the adaptive windowing policy.
func (store *Store) Step(number types.Number) types.Number {
	capacity := store.Adaptive.Step(float64(number))

	if capacity < MinimumStoreCapacity {
		capacity = MinimumStoreCapacity
	}

	store.Buffer = append(store.Buffer, number)

	if len(store.Buffer) > capacity {
		// Drop oldest samples to maintain emergent capacity
		excess := len(store.Buffer) - capacity
		store.Buffer = store.Buffer[excess:]
	}

	if store.Reduce == nil {
		// Algebraic Sink: returns 0 so parallel sum is uncorrupted (Law of Sinks)
		return 0
	}

	return store.Reduce(store.Buffer)
}

// Values returns the currently retained window of samples.
func (store *Store) Values() []types.Number {
	return store.Buffer
}

// Len returns the count of samples currently retained.
func (store *Store) Len() int {
	return len(store.Buffer)
}
