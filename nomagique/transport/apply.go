package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Apply binds a run to a target.
*/
type Apply struct {
	*core.PrimitiveError
	target core.Primitive
	bound  iter.Seq[unsafe.Pointer]
}

func NewApply(
	target core.Primitive,
	bound iter.Seq[unsafe.Pointer],
) core.Primitive {
	return &Apply{
		PrimitiveError: core.NewPrimitiveError(),
		target:         target,
		bound:          bound,
	}
}

func (op *Apply) Next(iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for out := range op.target.Next(op.bound) {
			if !yield(out) {
				return
			}
		}

		op.Error(op.target.Error())
	}
}
