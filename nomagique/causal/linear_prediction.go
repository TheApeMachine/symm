package causal

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
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
	err error
	dot core.Primitive
	out float64
}

func NewLinearPrediction() core.Primitive {
	return &LinearPrediction{dot: vector.NewDot()}
}

func (op *LinearPrediction) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*PredictionQuery)(arriving)
			designNode := vector.NewDesign(query.Features...)

			var design []float64

			for out := range designNode.Next(transport.NewValues(query.Row).Next(nil)) {
				copied := make([]float64, len(*(*[]float64)(out)))
				copy(copied, *(*[]float64)(out))
				design = copied
			}

			if err := designNode.Error(); err != nil {
				op.Error(err)
				return
			}

			pair := vector.Pair{
				Left:  query.Fit.Coefficients,
				Right: design,
			}

			for out := range op.dot.Next(transport.NewValues(pair).Next(nil)) {
				op.out = *(*float64)(out)
			}

			if err := op.dot.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *LinearPrediction) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
