package store

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Get owns lookup. Missing keys are explicit failures, never a fabricated zero.
*/
type Get[K comparable, V any] struct {
	*core.PrimitiveError

	key K
	out V
}

func NewGet[K comparable, V any](key K) *Get[K, V] {
	return &Get[K, V]{PrimitiveError: core.NewPrimitiveError(), key: key}
}

func (get *Get[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*map[K]V)(arriving)
			value, found := m[get.key]

			if !found {
				get.Error(fmt.Errorf("%w: key %v", core.ErrNotHeld, get.key))
				return
			}

			get.out = value

			if !yield(unsafe.Pointer(&get.out)) {
				return
			}
		}
	}
}
