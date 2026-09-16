package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"golang.design/x/lockfree"
)

/*
KV associates incoming keys and values with a configured map. It never mutates
that source: each arrival is merged into a private copy.
*/
type KV[K comparable, V any] struct {
	*core.PrimitiveError
	current lockfree.Map[K, V]
}

func NewKV[K comparable, V any](initial lockfree.Map[K, V]) *KV[K, V] {
	return &KV[K, V]{
		PrimitiveError: core.NewPrimitiveError(),
		current:        initial,
	}
}

func (kv *KV[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
		}
	}
}
