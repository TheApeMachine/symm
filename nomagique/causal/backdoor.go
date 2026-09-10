package causal

import (
	"iter"
	"math"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
BackdoorReading is a model-dependent interventional estimate, not a claim that
observational data identifies cause.
*/
type BackdoorReading struct {
	Defined     bool
	Expectation float64
}

/*
Backdoor standardizes configured predictions over the observed rows after
replacing the treatment coordinate.
*/
type Backdoor struct {
	core.Base[Query, BackdoorReading]
	fit     *LinearFit
	predict *LinearPrediction
}

func NewBackdoor(tolerance float64) *Backdoor {
	return &Backdoor{
		fit:     NewLinearFit(tolerance),
		predict: NewLinearPrediction(),
	}
}

func (op *Backdoor) Next(
	in iter.Seq[core.Primitive[Query, Query]],
) iter.Seq[core.Primitive[BackdoorReading, BackdoorReading]] {
	return func(yield func(core.Primitive[BackdoorReading, BackdoorReading]) bool) {
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

func (op *Backdoor) Estimate(query Query) (BackdoorReading, error) {
	fit, err := op.fit.Fit(query)

	if err != nil {
		return BackdoorReading{}, err
	}

	if !fit.Defined {
		return BackdoorReading{Expectation: math.NaN()}, nil
	}

	predictions := make([]float64, 0, len(query.Rows))

	for _, row := range query.Rows {
		intervened := slices.Clone(row)
		intervened[query.Treatment] = query.Level
		value, err := op.predict.Predict(PredictionQuery{
			Fit:      fit,
			Features: query.Features,
			Row:      intervened,
		})

		if err != nil {
			return BackdoorReading{}, err
		}

		predictions = append(predictions, value)
	}

	mean := equation.NewMean[float64]()
	expectation := 0.0

	for out := range mean.Next(transport.Values(predictions...)) {
		expectation = out.Read()
	}

	if err := mean.Error(); err != nil {
		return BackdoorReading{}, err
	}

	return BackdoorReading{Defined: true, Expectation: expectation}, nil
}
