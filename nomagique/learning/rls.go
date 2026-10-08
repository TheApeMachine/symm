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
yields SquareRootRLS's *[][]float64, whose row [0] {prediction, scale, degrees
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
	control   []float64
	query     [2][]float64
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
		control:        make([]float64, 2),
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

			op.control[0] = op.lambda
			op.query[0] = op.design
			op.query[1] = op.control[:1]

			if observed {
				op.control[1] = row[op.dimension]
				op.query[1] = op.control[:2]
			}

			for out := range op.learner.Next(data.NewValue(op.query).Next(nil)) {
				op.out = *(*[][]float64)(out)
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
