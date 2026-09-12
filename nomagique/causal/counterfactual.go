package causal

import (
	"errors"
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
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
	err     error
	fit     core.Primitive
	predict core.Primitive
	abs     core.Primitive
	out     CounterfactualReading
}

func NewCounterfactual(tolerance float64) core.Primitive {
	return &Counterfactual{
		fit:     NewLinearFit(tolerance),
		predict: NewLinearPrediction(),
		abs:     calculus.NewAbsolute(),
	}
}

func (op *Counterfactual) Next(
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

			for out := range op.fit.Next(transport.NewValues(*query).Next(nil)) {
				fit = *(*algo.Fit)(out)
			}

			if err := op.fit.Error(); err != nil {
				op.Error(err)
				return
			}

			if !fit.Defined {
				op.out = undefined

				if !yield(unsafe.Pointer(&op.out)) {
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

			for out := range op.predict.Next(transport.NewValues(factualPQ).Next(nil)) {
				factual = *(*float64)(out)
			}

			if err := op.predict.Error(); err != nil {
				op.Error(err)
				return
			}

			if query.Target < 0 || query.Target >= len(query.Actual) {
				op.Error(core.ErrShape)
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

			for out := range op.predict.Next(transport.NewValues(intervenedPQ).Next(nil)) {
				predicted = *(*float64)(out)
			}

			if err := op.predict.Error(); err != nil {
				op.Error(err)
				return
			}

			noise := outcome - factual
			absNoise := noise

			for out := range op.abs.Next(transport.NewValues(absNoise).Next(nil)) {
				absNoise = *(*float64)(out)
			}

			if err := op.abs.Error(); err != nil {
				op.Error(err)
				return
			}

			op.out = CounterfactualReading{
				Defined:        true,
				Noise:          noise,
				Counterfactual: predicted + noise,
				Precision:      1 / (1 + absNoise),
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Counterfactual) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
