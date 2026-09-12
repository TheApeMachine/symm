package store

import (
	"errors"
	"iter"
	"maps"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
KV associates incoming keys and values with a configured map. It never mutates
that source: each arrival is merged into a private copy.
*/
type KV[K comparable, V any] struct {
	err  error
	held map[K]V
	out  map[K]V
}

func NewKV[K comparable, V any](current map[K]V) core.Primitive {
	held := make(map[K]V, len(current))
	maps.Copy(held, current)

	return &KV[K, V]{held: held}
}

func (op *KV[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			arrived := *(*map[K]V)(arriving)
			merged := make(map[K]V, len(op.held)+len(arrived))
			maps.Copy(merged, op.held)
			maps.Copy(merged, arrived)
			op.held = merged
			op.out = merged

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *KV[K, V]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
