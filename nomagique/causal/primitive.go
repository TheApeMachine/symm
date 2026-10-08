package causal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algebra/linear"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Primitive is a Number pipeline that implements Judea Pearl's Causal Inference.
*/
type Primitive struct {
	*core.PrimitiveError
	matrix   *linear.Matrix
	pipeline *nomagique.Number
}

func NewPrimitive(rows, columns int) *Primitive {
	return &Primitive{
		PrimitiveError: core.NewPrimitiveError(),
		matrix:         linear.NewMatrix(rows, columns),
		pipeline: nomagique.NewNumber(
			NewBackdoor(1e-15),
		),
	}
}

func (op *Primitive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for out := range op.pipeline.Next(in) {
			if !yield(out) {
				return
			}
		}

		if err := op.pipeline.Error(); err != nil {
			op.Error(err)
			return
		}
	}
}
