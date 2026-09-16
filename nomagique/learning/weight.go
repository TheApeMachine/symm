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
TrustReading is the residual-range trust update. No clipping is added to the
trust recurrence; rate greater than one retains the original extrapolating
behavior.
*/
type TrustReading struct {
	Value float64
	Trust float64
	Rate  float64
	Count float64
}

/*
TrustWeight owns that recurrence. Invalid inputs fail before entering retained
state.
*/
type TrustWeight struct {
	*core.PrimitiveError

	span  core.Primitive
	mix   core.Primitive
	abs   core.Primitive
	count float64
	min   float64
	max   float64
	trust float64
	rate  float64
	prev  float64
	out   TrustReading
}

func NewTrustWeight() *TrustWeight {
	return &TrustWeight{PrimitiveError: core.NewPrimitiveError(), span: statistic.NewResidualSpan(),
		mix:   calculus.NewMix(),
		abs:   calculus.NewAbsolute(),
		trust: 1,
	}
}

func (trustWeight *TrustWeight) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if math.IsNaN(pair.Predicted) || math.IsNaN(pair.Actual) ||
				math.IsInf(pair.Predicted, 0) || math.IsInf(pair.Actual, 0) {
				trustWeight.Error(core.ErrDomain)
				return
			}

			residual := pair.Actual - pair.Predicted
			spanInput := statistic.ResidualSpanInput{
				Count:    trustWeight.count,
				Minimum:  trustWeight.min,
				Maximum:  trustWeight.max,
				Residual: residual,
			}

			var span statistic.ResidualSpanResult

			for out := range trustWeight.span.Next(sequence.NewValues(spanInput).Next(nil)) {
				span = *(*statistic.ResidualSpanResult)(out)
			}

			if err := trustWeight.span.Error(); err != nil {
				trustWeight.Error(err)
				return
			}

			trustWeight.count = span.Count
			trustWeight.min = span.Minimum
			trustWeight.max = span.Maximum

			if trustWeight.count > 1 {
				if !(span.Span > 0) {
					trustWeight.Error(core.ErrDomain)
					return
				}

				magnitude := residual

				for out := range trustWeight.abs.Next(sequence.NewValues(magnitude).Next(nil)) {
					magnitude = *(*float64)(out)
				}

				if err := trustWeight.abs.Error(); err != nil {
					trustWeight.Error(err)
					return
				}

				trustWeight.rate = magnitude / span.Span
				mixRec := calculus.MixRecord{
					Left:   trustWeight.trust,
					Right:  math.Max(0, 1-trustWeight.rate),
					Weight: trustWeight.rate,
				}

				var trust float64

				for out := range trustWeight.mix.Next(sequence.NewValues(mixRec).Next(nil)) {
					trust = *(*float64)(out)
				}

				if err := trustWeight.mix.Error(); err != nil {
					trustWeight.Error(err)
					return
				}

				trustWeight.trust = trust
				trustWeight.prev = pair.Predicted
			}

			trustWeight.out = TrustReading{
				Value: trustWeight.trust,
				Trust: trustWeight.trust,
				Rate:  trustWeight.rate,
				Count: trustWeight.count,
			}

			if !yield(unsafe.Pointer(&trustWeight.out)) {
				return
			}
		}
	}
}
