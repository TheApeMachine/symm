package store

import (
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Get owns lookup. Missing keys are explicit failures, never a fabricated zero.
*/
type Get[K comparable, V any] struct {
	core.Base[map[K]V, V]
	key K
}

func NewGet[K comparable, V any](key K) *Get[K, V] {
	return &Get[K, V]{key: key}
}

func (op *Get[K, V]) Next(
	in iter.Seq[core.Primitive[map[K]V, map[K]V]],
) iter.Seq[core.Primitive[V, V]] {
	return func(yield func(core.Primitive[V, V]) bool) {
		for arriving := range in {
			value, found := arriving.Read()[op.key]

			if !found {
				op.Error(fmt.Errorf("%w: key %v", core.ErrNotHeld, op.key))
				continue
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
