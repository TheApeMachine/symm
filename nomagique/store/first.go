package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
First is the leading member of a pair. Pairs travel as one value, so taking
one member is the smallest operation that takes them apart.
*/
type First[T any] struct {
	err error
	out T
}

func NewFirst[T any]() core.Primitive {
	return &First[T]{}
}

func (op *First[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			op.out = (*[2]T)(arriving)[0]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *First[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
