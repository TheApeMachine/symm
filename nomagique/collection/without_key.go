package collection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
WithoutKey excludes one configured key from each arriving keyed collection.
Each arrival is a *map[K]V; it yields a *map[K]V holding every other member,
and leaves the arriving map alone.
*/
type WithoutKey[K comparable, V any] struct {
	*core.PrimitiveError
	key K
	out map[K]V
}

func NewWithoutKey[K comparable, V any](key K) core.Primitive {
	return &WithoutKey[K, V]{
		PrimitiveError: core.NewPrimitiveError(),
		key:            key,
	}
}

func (op *WithoutKey[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			values := *(*map[K]V)(arriving)
			op.out = make(map[K]V, len(values))

			for key, value := range values {
				if key != op.key {
					op.out[key] = value
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
