package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Retained holds the latest arrival. Configuration supplies the value before the
first update.
*/
type Retained[T any] struct {
	err  error
	held T
}

func NewRetained[T any](current T) core.Primitive {
	return &Retained[T]{held: current}
}

func (op *Retained[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			yield(unsafe.Pointer(&op.held))
			return
		}

		for arriving := range in {
			op.held = *(*T)(arriving)

			if !yield(unsafe.Pointer(&op.held)) {
				return
			}
		}
	}
}

func (op *Retained[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
