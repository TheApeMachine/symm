package collection

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Set replaces one indexed member without mutating its input collection.
*/
type Set[T any] struct {
	err   error
	index int
	value T
	out   []T
}

func NewSet[T any](index int, value T) core.Primitive {
	return &Set[T]{index: index, value: value}
}

func (op *Set[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if op.index < 0 || op.index >= len(values) {
				op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, op.index, len(values)))
				return
			}

			updated := slices.Clone(values)
			updated[op.index] = op.value
			op.out = updated

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Set[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
