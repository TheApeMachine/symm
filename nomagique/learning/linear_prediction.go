package learning

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
PredictionQuery is one row evaluated against a fitted affine model.
*/
type PredictionQuery struct {
	Fit      algo.Fit
	Features []int
	Row      []float64
}

/*
LinearPrediction evaluates one row against a fit. It does not train.
*/
type LinearPrediction struct {
	*core.PrimitiveError

	dot core.Primitive
	out float64
}

func NewLinearPrediction() *LinearPrediction {
	return &LinearPrediction{PrimitiveError: core.NewPrimitiveError(), dot: arithmetic.NewDot()}
}

func (linearPrediction *LinearPrediction) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*PredictionQuery)(arriving)
			designNode := arithmetic.NewDesign(query.Features...)

			var design []float64

			for out := range designNode.Next(sequence.NewValues(query.Row).Next(nil)) {
				copied := make([]float64, len(*(*[]float64)(out)))
				copy(copied, *(*[]float64)(out))
				design = copied
			}

			if err := designNode.Error(); err != nil {
				linearPrediction.Error(err)
				return
			}

			pair := arithmetic.Pair{
				Left:  query.Fit.Coefficients,
				Right: design,
			}

			for out := range linearPrediction.dot.Next(sequence.NewValues(pair).Next(nil)) {
				linearPrediction.out = *(*float64)(out)
			}

			if err := linearPrediction.dot.Error(); err != nil {
				linearPrediction.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&linearPrediction.out)) {
				return
			}
		}
	}
}
