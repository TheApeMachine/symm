package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Constant replaces each arrival with a retained value. With one constructor
// operand it is fixed immediately. With no operand, its first scalar arrival
// establishes the value; later arrivals are clocks and never replace it.
type Constant[T any] struct {
	*core.PrimitiveError
	out  T
	held bool
}

func NewConstant[T any](current ...T) core.Primitive {
	op := &Constant[T]{PrimitiveError: core.NewPrimitiveError()}

	if len(current) > 1 {
		op.Error(core.ErrShape)
		return op
	}

	if len(current) == 1 {
		op.out = current[0]
		op.held = true
	}

	return op
}

func (op *Constant[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil || in == nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if !op.held {
				op.out = *(*T)(arriving)
				op.held = true
			}

			value := op.out

			if !yield(unsafe.Pointer(&value)) {
				return
			}
		}
	}
}
