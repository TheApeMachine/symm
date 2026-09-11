package learning

import (
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sample is one feature vector and an optional target. A features-only query
never trains.
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
	core.Base[Sample, algo.Reading]
	dimension int
	lambda    float64
	learner   *algo.SquareRootRLS
	design    []float64
}

func NewRLS(dimension int, variance, lambda float64) *RLS {
	design := make([]float64, dimension+1)
	design[0] = 1

	return &RLS{
		dimension: dimension,
		lambda:    lambda,
		learner:   algo.NewSquareRootRLS(variance),
		design:    design,
	}
}

func (op *RLS) Next(
	in iter.Seq[core.Primitive[Sample, Sample]],
) iter.Seq[core.Primitive[algo.Reading, algo.Reading]] {
	return func(yield func(core.Primitive[algo.Reading, algo.Reading]) bool) {
		for arriving := range in {
			reading, err := op.Prepare(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

/*
Prepare decodes a feature vector once and prepends the affine intercept.
*/
func (op *RLS) Prepare(sample Sample) (algo.Reading, error) {
	if op.dimension <= 0 || len(sample.Features) != op.dimension {
		return algo.Reading{}, fmt.Errorf(
			"%w: RLS expected %d features, received %d",
			core.ErrShape,
			op.dimension,
			len(sample.Features),
		)
	}

	design := make([]float64, len(sample.Features)+1)
	design[0] = 1
	copy(design[1:], sample.Features)

	return op.learner.Step(algo.Query{
		Design:   design,
		Target:   sample.Target,
		Observed: sample.Observed,
		Lambda:   op.lambda,
	})
}

/*
Predict evaluates the model on a feature vector without updating its weights.
It reuses the internal design vector to eliminate allocations on hot rollout paths.
*/
func (op *RLS) Predict(features []float64) (algo.Reading, error) {
	if op.dimension <= 0 || len(features) != op.dimension {
		return algo.Reading{}, fmt.Errorf(
			"%w: RLS expected %d features, received %d",
			core.ErrShape,
			op.dimension,
			len(features),
		)
	}

	if len(op.design) != op.dimension+1 {
		op.design = make([]float64, op.dimension+1)
	}
	op.design[0] = 1
	copy(op.design[1:], features)

	return op.learner.Step(algo.Query{
		Design:   op.design,
		Observed: false,
		Lambda:   op.lambda,
	})
}
