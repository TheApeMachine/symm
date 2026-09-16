package learning

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	*core.PrimitiveError

	fit     core.Primitive
	predict core.Primitive
	out     BackdoorReading
}

func NewBackdoor(tolerance float64) *Backdoor {
	return &Backdoor{PrimitiveError: core.NewPrimitiveError(), fit: NewLinearFit(tolerance),
		predict: NewLinearPrediction(),
	}
}

func (backdoor *Backdoor) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*Query)(arriving)

			var fit algo.Fit

			for out := range backdoor.fit.Next(sequence.NewValues(*query).Next(nil)) {
				fit = *(*algo.Fit)(out)
			}

			if err := backdoor.fit.Error(); err != nil {
				backdoor.Error(err)
				return
			}

			if !fit.Defined {
				backdoor.out = BackdoorReading{Expectation: math.NaN()}

				if !yield(unsafe.Pointer(&backdoor.out)) {
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

				for out := range backdoor.predict.Next(sequence.NewValues(pq).Next(nil)) {
					val = *(*float64)(out)
				}

				if err := backdoor.predict.Error(); err != nil {
					backdoor.Error(err)
					return
				}

				predictions = append(predictions, val)
			}

			meanNode := statistic.NewMean()
			var expectation float64

			for out := range meanNode.Next(sequence.NewValues(predictions...).Next(nil)) {
				expectation = *(*float64)(out)
			}

			if err := meanNode.Error(); err != nil {
				backdoor.Error(err)
				return
			}

			backdoor.out = BackdoorReading{
				Defined:     true,
				Expectation: expectation,
			}

			if !yield(unsafe.Pointer(&backdoor.out)) {
				return
			}
		}
	}
}
