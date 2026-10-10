package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
RLS supplies an affine intercept and the zero-mean diagonal coefficient prior.
Prediction is prior to the optional target's update, as owned by SquareRootRLS.

Each arrival is *[]float64. A row of exactly dimension features is a query and
never trains; a row of dimension+1 values carries the observed target as its
last element. Prediction and update share one arrival path: each arrival
yields *[][]float64 projected from SquareRootRLS's Reading, whose row [0] {prediction, scale, degrees
of freedom, predictive variance, ready, innovation, observed} is the
prequential prediction prior to that row's optional update, and whose rows
[1:] {beta, {noiseShape, noiseScale}, root...} are the committed posterior.
beta[0] is the intercept.
*/
type RLS struct {
	*core.PrimitiveError
	dimension int
	lambda    float64
	learner   core.Primitive
	out       [][]float64
	design    []float64
}

/*
NewRLS creates an RLS primitive over the given feature dimension, coefficient
prior variance, and forgetting factor.
*/
func NewRLS(dimension int, variance, lambda float64) core.Primitive {
	rls := &RLS{
		PrimitiveError: core.NewPrimitiveError(),
		dimension:      dimension,
		lambda:         lambda,
		learner:        algo.NewSquareRootRLS(variance),
	}

	if dimension > 0 {
		rls.design = make([]float64, dimension+1)
		rls.design[0] = 1
	}

	return rls
}

func (op *RLS) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			row := *(*[]float64)(arriving)
			observed := len(row) == op.dimension+1

			if op.dimension <= 0 || (len(row) != op.dimension && !observed) {
				op.Error(fmt.Errorf(
					"%w: RLS expected %d features (or %d with a target), received %d",
					core.ErrShape,
					op.dimension,
					op.dimension+1,
					len(row),
				))
				return
			}

			copy(op.design[1:], row[:op.dimension])

			query := algo.Query{Design: op.design, Lambda: op.lambda, Observed: observed}

			if observed {
				query.Target = row[op.dimension]
			}

			var reading *algo.Reading

			for out := range op.learner.Next(data.NewValue(query).Next(nil)) {
				reading = (*algo.Reading)(out)
			}

			if err := op.learner.Error(); err != nil {
				op.Error(err)
				return
			}

			if reading == nil {
				op.Error(fmt.Errorf("%w: RLS learner yielded no reading", core.ErrShape))
				return
			}

			op.project(reading)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
project lays one SquareRootRLS Reading out as the documented *[][]float64:
the prior forecast in row [0], then beta, the noise posterior, and the root.
*/
func (op *RLS) project(reading *algo.Reading) {
	ready, observed := 0.0, 0.0

	if reading.Ready {
		ready = 1
	}

	if reading.Observed {
		observed = 1
	}

	op.out = append(op.out[:0],
		[]float64{
			reading.Prediction, reading.Scale, reading.DegreesOfFreedom,
			reading.PredictiveVariance, ready, reading.Innovation, observed,
		},
		append([]float64(nil), reading.Beta...),
		[]float64{reading.NoiseShape, reading.NoiseScale},
	)

	for _, row := range reading.Root {
		op.out = append(op.out, append([]float64(nil), row...))
	}
}
