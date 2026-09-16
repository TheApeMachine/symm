package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Has owns key membership. Missing data can be routed before Get is applied;
lookup itself continues to reject absent keys rather than inventing a zero.
*/
type Has[K comparable, V any] struct {
	*core.PrimitiveError

	key K
	out bool
}

func NewHas[K comparable, V any](key K) *Has[K, V] {
	return &Has[K, V]{PrimitiveError: core.NewPrimitiveError(), key: key}
}

func (has *Has[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*map[K]V)(arriving)
			_, present := m[has.key]
			has.out = present

			if !yield(unsafe.Pointer(&has.out)) {
				return
			}
		}
	}
}
