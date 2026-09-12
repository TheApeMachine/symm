package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Constant replaces each arrival with a configured value. The arrival is the
clock; the payload is ignored.
*/
type Constant[T any] struct {
	err error
	out T
}

func NewConstant[T any](current T) core.Primitive {
	return &Constant[T]{out: current}
}

func (op *Constant[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for range in {
			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Constant[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
