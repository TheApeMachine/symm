package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algebra/linear"
	"github.com/theapemachine/symm/nomagique/core"
)

type Primitive struct {
	*core.PrimitiveError
	pipeline *nomagique.Number
}

func NewPrimitive(regions, channels int, weights []float64) *Primitive {
	return &Primitive{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline: nomagique.NewNumber(
			NewExtract(channels),
			linear.NewTransform(regions, channels, weights),
			NewDelta(regions),
			NewCondition(),
			NewEmit(regions),
		),
	}
}

/*
Next receives an iterator of *grid.Measurement, processes it, and yields
an iterator of *grid.Measurement.
*/
func (op *Primitive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return op.pipeline.Next(in)
}
