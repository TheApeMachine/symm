package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Has owns key membership. Missing data can be routed before Get is applied;
lookup itself continues to reject absent keys rather than inventing a zero.
*/
type Has[K comparable, V any] struct {
	err error
	key K
	out bool
}

func NewHas[K comparable, V any](key K) core.Primitive {
	return &Has[K, V]{key: key}
}

func (op *Has[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*map[K]V)(arriving)
			_, present := m[op.key]
			op.out = present

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Has[K, V]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
