package learning

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sample is one feature vector and an optional target. A features-only query
never trains, so prediction and update share one arrival path: the reading
each arrival yields is the prequential prediction prior to that sample's
optional update.
*/
type Sample struct {
	Features []float64
	Target   float64
	Observed bool
}

/*
RLS supplies an affine intercept and the zero-mean diagonal coefficient prior.
Prediction is prior to the optional target's update, as owned by SquareRootRLS.
*/
type RLS struct {
	err       error
	dimension int
	lambda    float64
	learner   core.Primitive
	out       algo.Reading
	design    []float64
	query     algo.Query
}

/*
NewRLS creates an RLS primitive over the given feature dimension, coefficient
prior variance, and forgetting factor.
*/
func NewRLS(dimension int, variance, lambda float64) core.Primitive {
	var design []float64
	if dimension > 0 {
		design = make([]float64, dimension+1)
		design[0] = 1
	}

	return &RLS{
		dimension: dimension,
		lambda:    lambda,
		learner:   algo.NewSquareRootRLS(variance),
		design:    design,
	}
}

func (op *RLS) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Sample)(arriving)

			if op.dimension <= 0 || len(sample.Features) != op.dimension {
				op.Error(fmt.Errorf(
					"%w: RLS expected %d features, received %d",
					core.ErrShape,
					op.dimension,
					len(sample.Features),
				))
				return
			}

			if len(op.design) != op.dimension+1 {
				op.design = make([]float64, op.dimension+1)
				op.design[0] = 1
			}

			copy(op.design[1:], sample.Features)

			op.query = algo.Query{
				Design:   op.design,
				Target:   sample.Target,
				Observed: sample.Observed,
				Lambda:   op.lambda,
			}

			for out := range op.learner.Next(func(y func(unsafe.Pointer) bool) {
				y(unsafe.Pointer(&op.query))
			}) {
				op.out = *(*algo.Reading)(out)
			}

			if err := op.learner.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *RLS) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
