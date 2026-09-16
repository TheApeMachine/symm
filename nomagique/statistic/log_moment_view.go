package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LogMomentView reads a log-space Welford record against prior moments. It does
not log the input again.
*/
type LogMomentView struct {
	*core.PrimitiveError

	out CausalResidualResult
}

func NewLogMomentView() *LogMomentView {
	return &LogMomentView{PrimitiveError: core.NewPrimitiveError()}
}

func (logMomentView *LogMomentView) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*MomentReading)(arriving)
			result := CausalResidualResult{
				MomentReading: *reading,
				HasPrior:      reading.Prior.Count > 0,
				Baseline:      math.Exp(reading.Prior.Mean),
				Residual:      reading.Value - reading.Prior.Mean,
			}

			if reading.Prior.Count > 1 && reading.Prior.M2 > 0 {
				result.PriorVariance = reading.Prior.M2 / (reading.Prior.Count - 1)
				result.ScoreScale = math.Sqrt(result.PriorVariance)
				result.ZScore = result.Residual / result.ScoreScale
			}

			logMomentView.out = result

			if !yield(unsafe.Pointer(&logMomentView.out)) {
				return
			}
		}
	}
}
