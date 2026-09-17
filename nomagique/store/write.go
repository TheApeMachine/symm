package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Write binds each arriving value to a configured key as a latest-store write.
*/
type Write[K comparable, V any] struct {
	*core.PrimitiveError

	key K
	out LatestCommand[K, V]
}

func NewWrite[K comparable, V any](key K) *Write[K, V] {
	return &Write[K, V]{PrimitiveError: core.NewPrimitiveError(), key: key}
}

func (write *Write[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			write.out = LatestCommand[K, V]{
				Key:   write.key,
				Value: *(*V)(arriving),
			}

			if !yield(unsafe.Pointer(&write.out)) {
				return
			}
		}
	}
}
