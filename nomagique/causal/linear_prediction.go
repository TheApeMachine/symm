package causal

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
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
	core.Base[PredictionQuery, float64]
	dot *vector.Dot
}

func NewLinearPrediction() *LinearPrediction {
	return &LinearPrediction{dot: vector.NewDot()}
}

func (op *LinearPrediction) Next(
	in iter.Seq[core.Primitive[PredictionQuery, PredictionQuery]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			value, err := op.Predict(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}

func (op *LinearPrediction) Predict(query PredictionQuery) (float64, error) {
	design, err := transport.Evaluate(equation.NewDesign[float64](query.Features), transport.Values(query.Row))

	if err != nil {
		return 0, err
	}

	return transport.Evaluate(op.dot, transport.Values(vector.Pair{
		Left:  query.Fit.Coefficients,
		Right: design,
	}))
}
