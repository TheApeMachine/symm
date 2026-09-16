package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
RatioReading preserves the supplied calibration ratio and observed-range
ceiling. Those are model policies, not statistical identities.
*/
type RatioReading struct {
	Value     float64
	PeakRatio float64
	Count     float64
}

/*
SampleRatio owns that policy over the shared residual-span update.
*/
type SampleRatio struct {
	*core.PrimitiveError

	span      core.Primitive
	abs       core.Primitive
	count     float64
	min       float64
	max       float64
	prev      float64
	peakRatio float64
	out       RatioReading
}

func NewSampleRatio() *SampleRatio {
	return &SampleRatio{PrimitiveError: core.NewPrimitiveError(), span: statistic.NewResidualSpan(),
		abs: calculus.NewAbsolute(),
	}
}

func (sampleRatio *SampleRatio) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if math.IsNaN(pair.Predicted) || math.IsNaN(pair.Actual) ||
				math.IsInf(pair.Predicted, 0) || math.IsInf(pair.Actual, 0) {
				sampleRatio.Error(core.ErrDomain)
				return
			}

			residual := pair.Actual - pair.Predicted
			spanInput := statistic.ResidualSpanInput{
				Count:    sampleRatio.count,
				Minimum:  sampleRatio.min,
				Maximum:  sampleRatio.max,
				Residual: residual,
			}

			var span statistic.ResidualSpanResult

			for out := range sampleRatio.span.Next(sequence.NewValues(spanInput).Next(nil)) {
				span = *(*statistic.ResidualSpanResult)(out)
			}

			if err := sampleRatio.span.Error(); err != nil {
				sampleRatio.Error(err)
				return
			}

			sampleRatio.count = span.Count
			sampleRatio.min = span.Minimum
			sampleRatio.max = span.Maximum
			ratio := pair.Actual / pair.Predicted

			if pair.Actual < pair.Predicted {
				ratio = 1 + pair.Actual/pair.Predicted
			}

			if !(pair.Predicted <= pair.Actual || ratio >= 0) {
				sampleRatio.Error(core.ErrDomain)
				return
			}

			ceiling := 1.0

			if span.Span > 0 {
				ceiling = 1 + 1/span.Span
			}

			if !(span.Span > 0) {
				prevAbs := sampleRatio.prev

				for out := range sampleRatio.abs.Next(sequence.NewValues(prevAbs).Next(nil)) {
					prevAbs = *(*float64)(out)
				}

				if err := sampleRatio.abs.Error(); err != nil {
					sampleRatio.Error(err)
					return
				}

				ceiling = 1 + 1/prevAbs
			}

			if ratio > ceiling {
				ratio = ceiling
			}

			if ratio > sampleRatio.peakRatio {
				sampleRatio.peakRatio = ratio
			}

			sampleRatio.prev = pair.Predicted
			sampleRatio.out = RatioReading{
				Value:     ratio,
				PeakRatio: sampleRatio.peakRatio,
				Count:     sampleRatio.count,
			}

			if !yield(unsafe.Pointer(&sampleRatio.out)) {
				return
			}
		}
	}
}
