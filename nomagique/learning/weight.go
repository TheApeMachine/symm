package learning

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
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
	core.Base[Pair, TrustReading]
	valid *equation.ValidPair[float64]
	span  *equation.ResidualSpan
	mix   *equation.Mix[float64]
	abs   *calculus.Absolute[float64]
	count float64
	min   float64
	max   float64
	trust float64
	rate  float64
	prev  float64
}

func NewTrustWeight() *TrustWeight {
	return &TrustWeight{
		valid: equation.NewValidPair[float64](),
		span:  equation.NewResidualSpan(),
		mix:   equation.NewMix[float64](),
		abs:   calculus.NewAbsolute[float64](),
		trust: 1,
	}
}

func (op *TrustWeight) Next(
	in iter.Seq[core.Primitive[Pair, Pair]],
) iter.Seq[core.Primitive[TrustReading, TrustReading]] {
	return func(yield func(core.Primitive[TrustReading, TrustReading]) bool) {
		for arriving := range in {
			reading, err := op.Measure(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *TrustWeight) Measure(pair Pair) (TrustReading, error) {
	ok, err := transport.Evaluate(op.valid, transport.Values(equation.ValidPairInput[float64]{
		Predicted: pair.Predicted,
		Actual:    pair.Actual,
	}))

	if err != nil {
		return TrustReading{}, err
	}

	if !ok {
		return TrustReading{}, core.ErrDomain
	}

	residual := pair.Actual - pair.Predicted
	span, err := transport.Evaluate(op.span, transport.Values(equation.ResidualSpanInput{
		Count:    op.count,
		Minimum:  op.min,
		Maximum:  op.max,
		Residual: residual,
	}))

	if err != nil {
		return TrustReading{}, err
	}

	op.count = span.Count
	op.min = span.Minimum
	op.max = span.Maximum

	if op.count > 1 {
		if !(span.Span > 0) {
			return TrustReading{}, core.ErrDomain
		}

		magnitude, err := transport.Evaluate(op.abs, transport.Values(residual))

		if err != nil {
			return TrustReading{}, err
		}

		op.rate = magnitude / span.Span
		trust, err := transport.Evaluate(op.mix, transport.Values(equation.MixRecord[float64]{
			Left:   op.trust,
			Right:  math.Max(0, 1-op.rate),
			Weight: op.rate,
		}))

		if err != nil {
			return TrustReading{}, err
		}

		op.trust = trust
		op.prev = pair.Predicted
	}

	return TrustReading{
		Value: op.trust,
		Trust: op.trust,
		Rate:  op.rate,
		Count: op.count,
	}, nil
}
