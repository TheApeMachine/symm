package causal

import (
	"errors"
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	err     error
	fit     core.Primitive
	predict core.Primitive
	out     BackdoorReading
}

func NewBackdoor(tolerance float64) core.Primitive {
	return &Backdoor{
		fit:     NewLinearFit(tolerance),
		predict: NewLinearPrediction(),
	}
}

func (op *Backdoor) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*Query)(arriving)

			var fit algo.Fit

			for out := range op.fit.Next(transport.NewValues(*query).Next(nil)) {
				fit = *(*algo.Fit)(out)
			}

			if err := op.fit.Error(); err != nil {
				op.Error(err)
				return
			}

			if !fit.Defined {
				op.out = BackdoorReading{Expectation: math.NaN()}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			predictions := make([]float64, 0, len(query.Rows))

			for _, row := range query.Rows {
				intervened := slices.Clone(row)
				intervened[query.Treatment] = query.Level

				pq := PredictionQuery{
					Fit:      fit,
					Features: query.Features,
					Row:      intervened,
				}

				var val float64

				for out := range op.predict.Next(transport.NewValues(pq).Next(nil)) {
					val = *(*float64)(out)
				}

				if err := op.predict.Error(); err != nil {
					op.Error(err)
					return
				}

				predictions = append(predictions, val)
			}

			meanNode := statistic.NewMean()
			var expectation float64

			for out := range meanNode.Next(transport.NewValues(predictions...).Next(nil)) {
				expectation = *(*float64)(out)
			}

			if err := meanNode.Error(); err != nil {
				op.Error(err)
				return
			}

			op.out = BackdoorReading{
				Defined:     true,
				Expectation: expectation,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Backdoor) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
