package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Has owns key membership. Missing data can be routed before Get is applied;
lookup itself continues to reject absent keys rather than inventing a zero.
*/
type Has[K comparable, V any] struct {
	core.Base[map[K]V, bool]
	key K
}

func NewHas[K comparable, V any](key K) *Has[K, V] {
	return &Has[K, V]{key: key}
}

func (op *Has[K, V]) Next(
	in iter.Seq[core.Primitive[map[K]V, map[K]V]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			_, present := arriving.Read()[op.key]

			if !yield(op.Carrier(present)) {
				return
			}
		}
	}
}
