package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Indexed is a value and where it fell in its run.
*/
type Indexed[T any] struct {
	Index int
	Value T
}

/*
Enumerate attaches a run-relative index to each value.
*/
type Enumerate[T any] struct {
	err error
	out Indexed[T]
}

func NewEnumerate[T any]() core.Primitive {
	return &Enumerate[T]{}
}

func (op *Enumerate[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		index := 0

		for arriving := range in {
			op.out = Indexed[T]{Index: index, Value: *(*T)(arriving)}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}

			index++
		}
	}
}

func (op *Enumerate[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
