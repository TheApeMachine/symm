package store

import (
	"iter"
	"maps"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
KV associates incoming keys and values with a configured map. It never mutates
that source: each arrival is merged into a private copy.
*/
type KV[K comparable, V any] struct {
	core.Base[map[K]V, map[K]V]
}

func NewKV[K comparable, V any](current map[K]V) *KV[K, V] {
	op := &KV[K, V]{}
	op.Carrier(current)
	return op
}

func (op *KV[K, V]) Next(
	in iter.Seq[core.Primitive[map[K]V, map[K]V]],
) iter.Seq[core.Primitive[map[K]V, map[K]V]] {
	return func(yield func(core.Primitive[map[K]V, map[K]V]) bool) {
		for arriving := range in {
			held := op.Read()
			merged := make(map[K]V, len(held)+len(arriving.Read()))
			maps.Copy(merged, held)
			maps.Copy(merged, arriving.Read())

			if !yield(op.Carrier(merged)) {
				return
			}
		}
	}
}
