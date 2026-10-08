package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Joint applies moment tracking and residual analysis per channel coordinate,
composing one Estimator per channel.

Each arrival is an observation vector as *[]float64 with one coordinate per
channel. It yields one *[]float64 reading, reused by the next arrival:

	[0] SNR, the average standardized energy over channels with energy
	[1] SNR defined (1 or 0)
	[2+8*c : 10+8*c] channel c: count, has prior (1 or 0), baseline (exp of
	    the prior log mean), prior variance, residual, score scale, z-score,
	    energy (z squared, NaN when the noise scale is indistinguishable)
*/
type Joint struct {
	*core.PrimitiveError
	estimators []core.Primitive
	out        []float64
}

func NewJoint(dimension int) core.Primitive {
	estimators := make([]core.Primitive, dimension)

	for index := range estimators {
		estimators[index] = NewEstimator()
	}

	return &Joint{
		PrimitiveError: core.NewPrimitiveError(),
		estimators:     estimators,
		out:            make([]float64, 2+8*dimension),
	}
}

func (op *Joint) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			values := *(*[]float64)(arriving)

			if len(values) != len(op.estimators) {
				op.Error(core.ErrShape)
				return
			}

			clear(op.out)
			energies := 0.0
			totalEnergy := 0.0

			for index, value := range values {
				estimator := op.estimators[index]
				var reading [10]float64

				for pointer := range estimator.Next(data.NewValue(value).Next(nil)) {
					reading = *(*[10]float64)(pointer)
				}

				if err := estimator.Error(); err != nil {
					op.Error(err)
					return
				}

				channel := op.out[2+8*index : 10+8*index]
				priorCount, priorMean, priorM2 := reading[3], reading[4], reading[5]
				channel[0] = reading[0]
				channel[2] = math.Exp(priorMean)
				channel[4] = value - priorMean
				channel[7] = math.NaN()

				if priorCount > 0 {
					channel[1] = 1
				}

				if priorCount > 1 && priorM2 > 0 {
					channel[3] = priorM2 / (priorCount - 1)
					channel[5] = math.Sqrt(channel[3])
					// Relative distinguishability: refuse energies when the noise
					// scale is below sqrt(eps)*max(1, |val|, |baseline|). Absolute
					// eps alone still admits billion-scale Z squared from
					// collapsed floors.
					ref := math.Max(1, math.Max(math.Abs(value), math.Abs(channel[2])))

					if channel[5] > math.Sqrt(2.220446049250313e-16)*ref {
						channel[6] = channel[4] / channel[5]

						if !math.IsInf(channel[6], 0) && !math.IsNaN(channel[6]) {
							channel[7] = channel[6] * channel[6]
							energies++
							totalEnergy += channel[7]
						}
					}
				}
			}

			if energies > 0 {
				op.out[0] = totalEnergy / energies
				op.out[1] = 1
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
