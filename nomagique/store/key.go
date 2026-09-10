package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Key associates each arrival with its configured key. Repeated writes to the
same key in a run retain the last value, exactly as KV does.
*/
type Key[K comparable, V any] struct {
	core.Base[V, map[K]V]
	key K
}

func NewKey[K comparable, V any](key K) *Key[K, V] {
	return &Key[K, V]{key: key}
}

func (op *Key[K, V]) Next(
	in iter.Seq[core.Primitive[V, V]],
) iter.Seq[core.Primitive[map[K]V, map[K]V]] {
	return func(yield func(core.Primitive[map[K]V, map[K]V]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(map[K]V{op.key: arriving.Read()})) {
				return
			}
		}
	}
}
