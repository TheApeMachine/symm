package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
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
	err       error
	span      core.Primitive
	abs       core.Primitive
	count     float64
	min       float64
	max       float64
	prev      float64
	peakRatio float64
	out       RatioReading
}

func NewSampleRatio() core.Primitive {
	return &SampleRatio{
		span: statistic.NewResidualSpan(),
		abs:  calculus.NewAbsolute(),
	}
}

func (op *SampleRatio) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if math.IsNaN(pair.Predicted) || math.IsNaN(pair.Actual) ||
				math.IsInf(pair.Predicted, 0) || math.IsInf(pair.Actual, 0) {
				op.Error(core.ErrDomain)
				return
			}

			residual := pair.Actual - pair.Predicted
			spanInput := statistic.ResidualSpanInput{
				Count:    op.count,
				Minimum:  op.min,
				Maximum:  op.max,
				Residual: residual,
			}

			var span statistic.ResidualSpanResult

			for out := range op.span.Next(transport.NewValues(spanInput).Next(nil)) {
				span = *(*statistic.ResidualSpanResult)(out)
			}

			if err := op.span.Error(); err != nil {
				op.Error(err)
				return
			}

			op.count = span.Count
			op.min = span.Minimum
			op.max = span.Maximum
			ratio := pair.Actual / pair.Predicted

			if pair.Actual < pair.Predicted {
				ratio = 1 + pair.Actual/pair.Predicted
			}

			if !(pair.Predicted <= pair.Actual || ratio >= 0) {
				op.Error(core.ErrDomain)
				return
			}

			ceiling := 1.0

			if span.Span > 0 {
				ceiling = 1 + 1/span.Span
			}

			if !(span.Span > 0) {
				prevAbs := op.prev

				for out := range op.abs.Next(transport.NewValues(prevAbs).Next(nil)) {
					prevAbs = *(*float64)(out)
				}

				if err := op.abs.Error(); err != nil {
					op.Error(err)
					return
				}

				ceiling = 1 + 1/prevAbs
			}

			if ratio > ceiling {
				ratio = ceiling
			}

			if ratio > op.peakRatio {
				op.peakRatio = ratio
			}

			op.prev = pair.Predicted
			op.out = RatioReading{
				Value:     ratio,
				PeakRatio: op.peakRatio,
				Count:     op.count,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *SampleRatio) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
