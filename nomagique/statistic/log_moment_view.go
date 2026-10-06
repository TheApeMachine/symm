package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LogMomentView reads a log-space Estimator reading (*[10]float64) against its
prior moments. It does not log the input again. It yields the CausalResidual
layout as *[8]float64, with the baseline mapped back out of log space:

	[0] has prior (1 or 0) [1] baseline    [2] prior variance [3] maturity (0)
	[4] residual           [5] score scale [6] z-score        [7] noise variance (0)
*/
type LogMomentView struct {
	*core.PrimitiveError
	out [8]float64
}

func NewLogMomentView() *LogMomentView {
	return &LogMomentView{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *LogMomentView) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
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

			op.out = [8]float64{}
			op.out[1] = math.Exp(priorMean)
			op.out[4] = reading[6] - priorMean

			if priorCount > 0 {
				op.out[0] = 1
			}

			if priorCount > 1 && priorM2 > 0 {
				op.out[2] = priorM2 / (priorCount - 1)
				op.out[5] = math.Sqrt(op.out[2])
				op.out[6] = op.out[4] / op.out[5]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
