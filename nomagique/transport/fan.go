package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fan presents one input run to every configured branch and streams what each
branch yields.
*/
type Fan struct {
	*core.PrimitiveError
	branches []core.Primitive
}

func NewFan(branches ...core.Primitive) core.Primitive {
	return &Fan{
		PrimitiveError: core.NewPrimitiveError(),
		branches:       branches,
	}
}

func (op *Fan) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, branch := range op.branches {
			for out := range branch.Next(in) {
				if !yield(out) {
					return
				}
			}

			op.Error(branch.Error())
		}
	}
}
