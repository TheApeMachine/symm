package learning

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Pair is a predicted-versus-actual observation.
*/
type Pair struct {
	Predicted float64
	Actual    float64
}

/*
ForecastReading is the learned multiplicative scale and its trust.
*/
type ForecastReading struct {
	Value       float64
	Scale       float64
	Trust       float64
	Rate        float64
	Count       float64
	WeightCount float64
}

/*
Forecast owns residual moments and the trust/scale recurrences. Mix supplies
both interpolations. The current residual participates in its surprise
statistic.
*/
type Forecast struct {
	core.Base[Pair, ForecastReading]
	valid   *equation.ValidPair[float64]
	welford *equation.Welford
	mix     *equation.Mix[float64]
	abs     *calculus.Absolute[float64]
	log     *calculus.Log[float64]
	exp     *calculus.Exp[float64]
	sqrt    *calculus.Sqrt[float64]
	finite  *logic.Finite[float64]
	trust   float64
	scale   float64
	rate    float64
}

func NewForecast() *Forecast {
	return &Forecast{
		valid:   equation.NewValidPair[float64](),
		welford: equation.NewWelford(),
		mix:     equation.NewMix[float64](),
		abs:     calculus.NewAbsolute[float64](),
		log:     calculus.NewLog[float64](),
		exp:     calculus.NewExp[float64](),
		sqrt:    calculus.NewSqrt[float64](),
		finite:  logic.NewFinite[float64](),
		trust:   1,
		scale:   1,
	}
}

func (op *Forecast) Next(
	in iter.Seq[core.Primitive[Pair, Pair]],
) iter.Seq[core.Primitive[ForecastReading, ForecastReading]] {
	return func(yield func(core.Primitive[ForecastReading, ForecastReading]) bool) {
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

func (op *Forecast) Measure(pair Pair) (ForecastReading, error) {
	ok, err := transport.Evaluate(op.valid, transport.Values(equation.ValidPairInput[float64]{
		Predicted: pair.Predicted,
		Actual:    pair.Actual,
	}))

	if err != nil {
		return ForecastReading{}, err
	}

	if !ok {
		return ForecastReading{}, core.ErrDomain
	}

	residual := pair.Actual - pair.Predicted
	moments, err := transport.Evaluate(op.welford, transport.Values(residual))

	if err != nil {
		return ForecastReading{}, err
	}

	if moments.Count > 1 {
		op.rate = 0

		if moments.Variance > 0 {
			deviation, err := transport.Evaluate(op.abs, transport.Values(residual-moments.Mean))

			if err != nil {
				return ForecastReading{}, err
			}

			spread, err := transport.Evaluate(op.sqrt, transport.Values(moments.Variance))

			if err != nil {
				return ForecastReading{}, err
			}

			op.rate = deviation / spread
		}

		trust, err := transport.Evaluate(op.mix, transport.Values(equation.MixRecord[float64]{
			Left:   op.trust,
			Right:  math.Max(0, 1-op.rate),
			Weight: op.rate,
		}))

		if err != nil {
			return ForecastReading{}, err
		}

		op.trust = trust
		actualAbs, err := transport.Evaluate(op.abs, transport.Values(pair.Actual))

		if err != nil {
			return ForecastReading{}, err
		}

		predictedAbs, err := transport.Evaluate(op.abs, transport.Values(pair.Predicted))

		if err != nil {
			return ForecastReading{}, err
		}

		actualLog, err := transport.Evaluate(op.log, transport.Values(actualAbs))

		if err != nil {
			return ForecastReading{}, err
		}

		predictedLog, err := transport.Evaluate(op.log, transport.Values(predictedAbs))

		if err != nil {
			return ForecastReading{}, err
		}

		target, err := transport.Evaluate(op.exp, transport.Values(actualLog-predictedLog))

		if err != nil {
			return ForecastReading{}, err
		}

		scale, err := transport.Evaluate(op.mix, transport.Values(equation.MixRecord[float64]{
			Left:   op.scale,
			Right:  target,
			Weight: op.rate * (1 - op.trust),
		}))

		if err != nil {
			return ForecastReading{}, err
		}

		op.scale = scale
	}

	defined, err := transport.Evaluate(op.finite, transport.Values(op.scale))

	if err != nil {
		return ForecastReading{}, err
	}

	if !defined {
		return ForecastReading{}, core.ErrDomain
	}

	return ForecastReading{
		Value:       op.scale,
		Scale:       op.scale,
		Trust:       op.trust,
		Rate:        op.rate,
		Count:       moments.Count,
		WeightCount: moments.Count,
	}, nil
}
