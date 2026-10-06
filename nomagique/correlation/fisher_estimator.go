package correlation

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
FisherEstimator transforms admissible scalar correlations through atanh,
tracks online moments, and computes causal residuals. Each arrival is
*float64; it yields
[10]float64{correlation, defined, baseline, divergence, priorCount, count,
zScore, variance, varianceDefined, hasPrior}. Invalid observations do not
advance the estimator.
*/
type FisherEstimator struct {
	*core.PrimitiveError
	moments  core.Primitive
	residual core.Primitive
	out      [10]float64
}

func NewFisherEstimator() core.Primitive {
	return &FisherEstimator{
		PrimitiveError: core.NewPrimitiveError(),
		moments:        statistic.NewEstimator(),
		residual:       statistic.NewCausalResidual(),
	}
}

func (op *FisherEstimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)
			op.out = [10]float64{value}

			if value > -1.0 && value < 1.0 {
				z := math.Atanh(value)
				var reading [10]float64
				var res [8]float64

				for pointer := range op.moments.Next(data.NewValue(z)) {
					reading = *(*[10]float64)(pointer)

					for out := range op.residual.Next(data.NewValue(reading)) {
						res = *(*[8]float64)(out)
					}
				}

				if err := errors.Join(op.moments.Error(), op.residual.Error()); err != nil {
					op.Error(err)
					return
				}

				priorCount := reading[3]

				op.out[1] = 1
				op.out[2] = math.Tanh(res[1])
				op.out[3] = res[4]
				op.out[4] = priorCount
				op.out[5] = reading[0]
				op.out[6] = res[6]

				if priorCount > 1 && res[2] > 0 {
					disp := math.Sqrt(res[2])
					ref := math.Abs(res[4])

					if ref < 1 {
						ref = 1
					}

					if disp > math.Sqrt(2.220446049250313e-16)*ref {
						op.out[7] = res[2]
						op.out[8] = 1
					}
				}

				if res[0] == 1 {
					op.out[9] = 1
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
