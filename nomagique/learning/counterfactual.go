package learning

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
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
	*core.PrimitiveError

	fit     core.Primitive
	predict core.Primitive
	abs     core.Primitive
	out     CounterfactualReading
}

func NewCounterfactual(tolerance float64) *Counterfactual {
	return &Counterfactual{PrimitiveError: core.NewPrimitiveError(), fit: NewLinearFit(tolerance),
		predict: NewLinearPrediction(),
		abs:     calculus.NewAbsolute(),
	}
}

func (counterfactual *Counterfactual) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*Query)(arriving)
			undefined := CounterfactualReading{
				Noise:          math.NaN(),
				Counterfactual: math.NaN(),
				Precision:      math.NaN(),
			}

			var fit algo.Fit

			for out := range counterfactual.fit.Next(sequence.NewValues(*query).Next(nil)) {
				fit = *(*algo.Fit)(out)
			}

			if err := counterfactual.fit.Error(); err != nil {
				counterfactual.Error(err)
				return
			}

			if !fit.Defined {
				counterfactual.out = undefined

				if !yield(unsafe.Pointer(&counterfactual.out)) {
					return
				}

				continue
			}

			factualPQ := PredictionQuery{
				Fit:      fit,
				Features: query.Features,
				Row:      query.Actual,
			}
			var factual float64

			for out := range counterfactual.predict.Next(sequence.NewValues(factualPQ).Next(nil)) {
				factual = *(*float64)(out)
			}

			if err := counterfactual.predict.Error(); err != nil {
				counterfactual.Error(err)
				return
			}

			if query.Target < 0 || query.Target >= len(query.Actual) {
				counterfactual.Error(core.ErrShape)
				return
			}

			outcome := query.Actual[query.Target]

			intervened := slices.Clone(query.Actual)
			intervened[query.Treatment] = query.Level

			intervenedPQ := PredictionQuery{
				Fit:      fit,
				Features: query.Features,
				Row:      intervened,
			}
			var predicted float64

			for out := range counterfactual.predict.Next(sequence.NewValues(intervenedPQ).Next(nil)) {
				predicted = *(*float64)(out)
			}

			if err := counterfactual.predict.Error(); err != nil {
				counterfactual.Error(err)
				return
			}

			noise := outcome - factual
			absNoise := noise

			for out := range counterfactual.abs.Next(sequence.NewValues(absNoise).Next(nil)) {
				absNoise = *(*float64)(out)
			}

			if err := counterfactual.abs.Error(); err != nil {
				counterfactual.Error(err)
				return
			}

			counterfactual.out = CounterfactualReading{
				Defined:        true,
				Noise:          noise,
				Counterfactual: predicted + noise,
				Precision:      1 / (1 + absNoise),
			}

			if !yield(unsafe.Pointer(&counterfactual.out)) {
				return
			}
		}
	}
}
