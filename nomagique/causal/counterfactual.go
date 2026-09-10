package causal

import (
	"iter"
	"math"
	"slices"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
CounterfactualReading preserves the factual residual. Precision is the source's
reconstruction audit weight 1/(1+|noise|), not a causal probability.
*/
type CounterfactualReading struct {
	Defined        bool
	Noise          float64
	Counterfactual float64
	Precision      float64
}

/*
Counterfactual composes abduction, intervention and prediction.
*/
type Counterfactual struct {
	core.Base[Query, CounterfactualReading]
	fit     *LinearFit
	predict *LinearPrediction
	abs     *calculus.Absolute[float64]
}

func NewCounterfactual(tolerance float64) *Counterfactual {
	return &Counterfactual{
		fit:     NewLinearFit(tolerance),
		predict: NewLinearPrediction(),
		abs:     calculus.NewAbsolute[float64](),
	}
}

func (op *Counterfactual) Next(
	in iter.Seq[core.Primitive[Query, Query]],
) iter.Seq[core.Primitive[CounterfactualReading, CounterfactualReading]] {
	return func(yield func(core.Primitive[CounterfactualReading, CounterfactualReading]) bool) {
		for arriving := range in {
			reading, err := op.Estimate(arriving.Read())

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

func (op *Counterfactual) Estimate(query Query) (CounterfactualReading, error) {
	undefined := CounterfactualReading{
		Noise:          math.NaN(),
		Counterfactual: math.NaN(),
		Precision:      math.NaN(),
	}
	fit, err := op.fit.Fit(query)

	if err != nil {
		return CounterfactualReading{}, err
	}

	if !fit.Defined {
		return undefined, nil
	}

	factual, err := op.predict.Predict(PredictionQuery{
		Fit:      fit,
		Features: query.Features,
		Row:      query.Actual,
	})

	if err != nil {
		return CounterfactualReading{}, err
	}

	outcome, err := transport.Evaluate(collection.NewAt[float64](query.Target), transport.Values(query.Actual))

	if err != nil {
		return CounterfactualReading{}, err
	}

	intervened := slices.Clone(query.Actual)
	intervened[query.Treatment] = query.Level
	predicted, err := op.predict.Predict(PredictionQuery{
		Fit:      fit,
		Features: query.Features,
		Row:      intervened,
	})

	if err != nil {
		return CounterfactualReading{}, err
	}

	noise := outcome - factual
	magnitude, err := transport.Evaluate(op.abs, transport.Values(noise))

	if err != nil {
		return CounterfactualReading{}, err
	}

	return CounterfactualReading{
		Defined:        true,
		Noise:          noise,
		Counterfactual: predicted + noise,
		Precision:      1 / (1 + magnitude),
	}, nil
}
