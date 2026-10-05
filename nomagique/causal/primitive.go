package causal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algebra/linear"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Primitive is a Number pipeline that implements Judea Pearl's Causal Inference.
*/
type Primitive struct {
	*core.PrimitiveError
	matrix   *linear.Matrix
	pipeline *nomagique.Number
	input    data.Map[string]
	output   data.Map[float64]
}

/*
NewPrimitive initializes a causal inference primitive.

Args:

	rows: The number of rows in the matrix.
	columns: The number of columns in the matrix.
*/
func NewPrimitive(rows, columns int) *Primitive {
	output := data.NewOutputMap()
	output.Values["expectation"] = 0
	output.Values["counterfactual"] = 0
	output.Values["noise"] = 0
	output.Values["precision"] = 0
	output.Values["defined"] = 0

	return &Primitive{
		PrimitiveError: core.NewPrimitiveError(),
		matrix:         linear.NewMatrix(rows, columns),
		pipeline: nomagique.NewNumber(
			NewBackdoor(1e-15),
			NewCounterfactual(1e-15),
		),
		input: data.NewMap(
			"level", "level",
			"baseline", "baseline",
			"effect", "effect",
			"actual", "actual",
			"factual", "factual",
			"predicted", "predicted",
		),
		output: output,
	}
}

/*
Next forwards causal inference results through the pipeline.

Args:

	in: An iterator of causal inference results.

Returns:

	An iterator of causal inference results.
*/
func (op *Primitive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			once := func(forward func(unsafe.Pointer) bool) {
				forward(arriving)
			}

			for out := range op.pipeline.Next(once) {
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
}
