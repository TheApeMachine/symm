package collection

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Append owns extending a collection.
*/
type Append[T any] struct {
	err  error
	held []T
	out  []T
}

func NewAppend[T any](current []T) core.Primitive {
	return &Append[T]{
		held: append([]T(nil), current...),
	}
}

func (op *Append[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			op.held = append(op.held, *(*T)(arriving))
			op.out = op.held

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Append[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
