package causal

import (
	"iter"
	"unsafe"

	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LinearFit estimates ordinary least squares parameters for arriving observations.
*/
type LinearFit struct {
	*core.PrimitiveError
	tolerance float64
}

func NewLinearFit(tolerance float64) core.Primitive {
	return &LinearFit{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
	}
}

func (op *LinearFit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 2 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 2 {
			op.Error(core.ErrShape)
			return
		}

		target := values[0]
		feature := values[1]

		design := mat.NewDense(1, 2, []float64{1.0, feature})
		outcome := mat.NewDense(1, 1, []float64{target})

		var solved mat.Dense
		solveErr := solved.Solve(design, outcome)

		var (
			intercept   float64
			coefficient float64
			defined     float64
		)

		if solveErr == nil {
			intercept = solved.At(0, 0)
			coefficient = solved.At(1, 0)
			defined = 1.0
		}

		for value := range data.NewValue(intercept, coefficient, defined).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
