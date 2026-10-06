package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CausalResidual measures an observation against the moments that existed
before it. It arrives as the Estimator's *[10]float64 reading and yields one
*[8]float64:

	[0] has prior (1 or 0) [1] baseline   [2] prior variance [3] maturity
	[4] residual           [5] score scale [6] z-score       [7] noise variance
*/
type CausalResidual struct {
	*core.PrimitiveError
	out [8]float64
}

func NewCausalResidual() *CausalResidual {
	return &CausalResidual{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *CausalResidual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			reading := (*[10]float64)(arriving)
			priorCount := reading[3]
			priorMean := reading[4]
			priorM2 := reading[5]
			value := reading[6]

			op.out = [8]float64{}
			op.out[1] = value
			op.out[3] = 1 - 1/(priorCount+1)
			op.out[7] = reading[8]

			if priorCount > 0 {
				op.out[0] = 1
				op.out[1] = priorMean
			}

			if priorCount > 1 {
				op.out[2] = priorM2 / (priorCount - 1)
			}

			op.out[4] = value - op.out[1]
			op.out[5] = math.Abs(op.out[4])

			if op.out[2] > 0 {
				dispersion := math.Sqrt(op.out[2])

				if dispersion > 2.220446049250313e-16 {
					op.out[5] = dispersion
				}
			}

			if op.out[5] > 0 {
				op.out[6] = op.out[4] / op.out[5]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
