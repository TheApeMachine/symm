package collection

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
At selects an indexed member.
*/
type At[T any] struct {
	err   error
	index int
	out   T
}

func NewAt[T any](index int) core.Primitive {
	return &At[T]{index: index}
}

func (op *At[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if op.index < 0 || op.index >= len(values) {
				op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, op.index, len(values)))
				return
			}

			op.out = values[op.index]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *At[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
