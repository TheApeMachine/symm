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
	err   error
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

func NewTrustWeight() core.Primitive {
	return &TrustWeight{
		span:  statistic.NewResidualSpan(),
		mix:   calculus.NewMix(),
		abs:   calculus.NewAbsolute(),
		trust: 1,
	}
}

func (op *TrustWeight) Next(
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

			if op.count > 1 {
				if !(span.Span > 0) {
					op.Error(core.ErrDomain)
					return
				}

				magnitude := residual

				for out := range op.abs.Next(transport.NewValues(magnitude).Next(nil)) {
					magnitude = *(*float64)(out)
				}

				if err := op.abs.Error(); err != nil {
					op.Error(err)
					return
				}

				op.rate = magnitude / span.Span
				mixRec := calculus.MixRecord{
					Left:   op.trust,
					Right:  math.Max(0, 1-op.rate),
					Weight: op.rate,
				}

				var trust float64

				for out := range op.mix.Next(transport.NewValues(mixRec).Next(nil)) {
					trust = *(*float64)(out)
				}

				if err := op.mix.Error(); err != nil {
					op.Error(err)
					return
				}

				op.trust = trust
				op.prev = pair.Predicted
			}

			op.out = TrustReading{
				Value: op.trust,
				Trust: op.trust,
				Rate:  op.rate,
				Count: op.count,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *TrustWeight) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
