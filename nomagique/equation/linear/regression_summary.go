package linear

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
)

/*
Summary is the cumulative slope and residual SNR. Undefined domains publish
zero with an explicit flag; no ridge is substituted.
*/
type Summary struct {
	Slope        float64
	SlopeDefined bool
	SNR          float64
	SNRDefined   bool
	Count        float64
}

/*
RegressionSummary projects moments into slope and SNR.
*/
type RegressionSummary struct {
	core.Base[Moments, Summary]
	finite *logic.Finite[float64]
}

func NewRegressionSummary() *RegressionSummary {
	return &RegressionSummary{finite: logic.NewFinite[float64]()}
}

func (op *RegressionSummary) Next(
	in iter.Seq[core.Primitive[Moments, Moments]],
) iter.Seq[core.Primitive[Summary, Summary]] {
	return func(yield func(core.Primitive[Summary, Summary]) bool) {
		for arriving := range in {
			moments := arriving.Read()
			summary := Summary{Count: moments.Count}

			if moments.Count == 0 {
				if !yield(op.Carrier(summary)) {
					return
				}

				continue
			}

			meanX := moments.SumX / moments.Count
			meanY := moments.SumY / moments.Count
			sxx := moments.SumXX - moments.Count*meanX*meanX
			sxy := moments.SumXY - moments.Count*meanX*meanY
			syy := moments.SumYY - moments.Count*meanY*meanY
			slope := sxy / sxx
			sse := syy - slope*sxy
			residual := sse / (moments.Count - 2)
			snr := (slope * slope) / (residual / sxx)

			summary.SlopeDefined = moments.Count >= 3 && sxx > 0 && defined(op.finite, slope)
			summary.SNRDefined = moments.Count >= 4 && sxx > 0 && syy > 0 && sse > 0 && residual > 0 && defined(op.finite, snr)

			if summary.SlopeDefined {
				summary.Slope = slope
			}

			if summary.SNRDefined {
				summary.SNR = snr
			}

			if !yield(op.Carrier(summary)) {
				return
			}
		}
	}
}
