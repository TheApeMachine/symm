package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LagProfile owns the configured estimator and exact discrete search coordinates.
Each arrival is *[2][][2]float64{leftPrices, rightPrices}; for each lag index
it yields [10]float64{correlation, covariance, support, leftEnergy, rightEnergy,
defined, index, lagIndex, x, y}.
*/
type LagProfile struct {
	*core.PrimitiveError
	estimator    core.Primitive
	leftReturns  core.Primitive
	rightReturns core.Primitive
	spacing      float64
	span         float64
	query        [3][]float64
	out          [10]float64
}

func NewLagProfile(estimator core.Primitive, spacing float64, span float64) core.Primitive {
	return &LagProfile{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
		leftReturns:    NewReturns(),
		rightReturns:   NewReturns(),
		spacing:        spacing,
		span:           span,
	}
}

func (op *LagProfile) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[2][][2]float64)(arriving)
			var left, right [2][]float64

			for pointer := range op.leftReturns.Next(data.NewValue(input[0])) {
				left = *(*[2][]float64)(pointer)
			}

			if err := op.leftReturns.Error(); err != nil {
				op.Error(err)
				return
			}

			for pointer := range op.rightReturns.Next(data.NewValue(input[1])) {
				right = *(*[2][]float64)(pointer)
			}

			if err := op.rightReturns.Error(); err != nil {
				op.Error(err)
				return
			}

			leftEnergy, rightEnergy := 0.0, 0.0

			if len(left[1]) > 0 {
				leftEnergy = left[1][0]
			}

			if len(right[1]) > 0 {
				rightEnergy = right[1][0]
			}

			op.query[0] = left[0]
			op.query[1] = right[0]
			op.query[2] = []float64{leftEnergy, rightEnergy, 0}

			limit := int(op.span*2 + 1)

			for index := 0; index < limit; index++ {
				lagIndex := float64(index) - op.span
				lag := lagIndex * op.spacing
				op.query[2][2] = lag

				var reading [6]float64

				for pointer := range op.estimator.Next(data.NewValue(op.query)) {
					reading = *(*[6]float64)(pointer)
				}

				if err := op.estimator.Error(); err != nil {
					op.Error(err)
					return
				}

				op.out = [10]float64{
					reading[0],
					reading[1],
					reading[2],
					reading[3],
					reading[4],
					reading[5],
					float64(index),
					lagIndex,
					lag * 1e-9,
					reading[0],
				}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}
			}
		}
	}
}
