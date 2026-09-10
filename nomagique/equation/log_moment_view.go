package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LogMomentView reads a log-space Welford record against prior moments. It does
not log the input again.
*/
type LogMomentView struct {
	core.Base[MomentReading, CausalResidualResult]
}

func NewLogMomentView() *LogMomentView {
	return &LogMomentView{}
}

func (op *LogMomentView) Next(
	in iter.Seq[core.Primitive[MomentReading, MomentReading]],
) iter.Seq[core.Primitive[CausalResidualResult, CausalResidualResult]] {
	return func(yield func(core.Primitive[CausalResidualResult, CausalResidualResult]) bool) {
		for arriving := range in {
			reading := arriving.Read()
			result := CausalResidualResult{
				MomentReading: reading,
				HasPrior:      reading.Prior.Count > 0,
				Baseline:      math.Exp(reading.Prior.Mean),
				Residual:      reading.Value - reading.Prior.Mean,
			}

			if reading.Prior.Count > 1 && reading.Prior.M2 > 0 {
				result.PriorVariance = reading.Prior.M2 / (reading.Prior.Count - 1)
				result.ScoreScale = math.Sqrt(result.PriorVariance)
				result.ZScore = result.Residual / result.ScoreScale
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}
