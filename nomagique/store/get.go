package store

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Get owns lookup. Missing keys are explicit failures, never a fabricated zero.
*/
type Get[K comparable, V any] struct {
	err error
	key K
	out V
}

func NewGet[K comparable, V any](key K) core.Primitive {
	return &Get[K, V]{key: key}
}

func (op *Get[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*map[K]V)(arriving)
			value, found := m[op.key]

			if !found {
				op.Error(fmt.Errorf("%w: key %v", core.ErrNotHeld, op.key))
				return
			}

			op.out = value

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Get[K, V]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
