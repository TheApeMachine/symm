package learning

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
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
	core.Base[Pair, RatioReading]
	valid     *equation.ValidPair[float64]
	span      *equation.ResidualSpan
	abs       *calculus.Absolute[float64]
	count     float64
	min       float64
	max       float64
	prev      float64
	peakRatio float64
}

func NewSampleRatio() *SampleRatio {
	return &SampleRatio{
		valid: equation.NewValidPair[float64](),
		span:  equation.NewResidualSpan(),
		abs:   calculus.NewAbsolute[float64](),
	}
}

func (op *SampleRatio) Next(
	in iter.Seq[core.Primitive[Pair, Pair]],
) iter.Seq[core.Primitive[RatioReading, RatioReading]] {
	return func(yield func(core.Primitive[RatioReading, RatioReading]) bool) {
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

func (op *SampleRatio) Measure(pair Pair) (RatioReading, error) {
	ok, err := transport.Evaluate(op.valid, transport.Values(equation.ValidPairInput[float64]{
		Predicted: pair.Predicted,
		Actual:    pair.Actual,
	}))

	if err != nil {
		return RatioReading{}, err
	}

	if !ok {
		return RatioReading{}, core.ErrDomain
	}

	residual := pair.Actual - pair.Predicted
	span, err := transport.Evaluate(op.span, transport.Values(equation.ResidualSpanInput{
		Count:    op.count,
		Minimum:  op.min,
		Maximum:  op.max,
		Residual: residual,
	}))

	if err != nil {
		return RatioReading{}, err
	}

	op.count = span.Count
	op.min = span.Minimum
	op.max = span.Maximum
	ratio := pair.Actual / pair.Predicted

	if pair.Actual < pair.Predicted {
		ratio = 1 + pair.Actual/pair.Predicted
	}

	if !(pair.Predicted <= pair.Actual || ratio >= 0) {
		return RatioReading{}, core.ErrDomain
	}

	ceiling := 1.0

	if span.Span > 0 {
		ceiling = 1 + 1/span.Span
	}

	if !(span.Span > 0) {
		prevAbs, err := transport.Evaluate(op.abs, transport.Values(op.prev))

		if err != nil {
			return RatioReading{}, err
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
	return RatioReading{Value: ratio, PeakRatio: op.peakRatio, Count: op.count}, nil
}
