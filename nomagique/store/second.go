package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Second is the trailing member of a pair, and the counterpart of First.
*/
type Second[T any] struct {
	err error
	out T
}

func NewSecond[T any]() core.Primitive {
	return &Second[T]{}
}

func (op *Second[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			op.out = (*[2]T)(arriving)[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Second[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
