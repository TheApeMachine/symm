package correlation

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Relative tracks causal baseline, residual divergence, and z-score of the
log relative return energy y_E = log(R_E) using Welford moments and causal residuals.
Input arrives as relativeEnergy (float64).
Yields values one by one: baseline, divergence, zscore, defined.
*/
type Relative struct {
	*core.PrimitiveError
	moments  core.Primitive
	residual core.Primitive
}

func NewRelative() core.Primitive {
	return &Relative{
		PrimitiveError: core.NewPrimitiveError(),
		moments:        statistic.NewEstimator(),
		residual:       statistic.NewCausalResidual(),
	}
}

func (op *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var relativeEnergy float64
		hasValue := false

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			relativeEnergy = *(*float64)(arriving)
			hasValue = true
		}

		if !hasValue {
			op.Error(core.ErrShape)
			return
		}

		if relativeEnergy <= 0 {
			for value := range data.NewValue(0.0, 0.0, 0.0, 0.0).Next(nil) {
				if !yield(value) {
					return
				}
			}

			return
		}

		logEnergy := math.Log(relativeEnergy)
		var reading [10]float64
		var res [8]float64

		for pointer := range op.moments.Next(data.NewValue(logEnergy).Next(nil)) {
			reading = *(*[10]float64)(pointer)

			for out := range op.residual.Next(data.NewValue(reading).Next(nil)) {
				res = *(*[8]float64)(out)
			}
		}

		if err := errors.Join(op.moments.Error(), op.residual.Error()); err != nil {
			op.Error(err)
			return
		}

		baseline := 0.0
		divergence := 0.0
		zscore := 0.0
		defined := 1.0

		if res[0] == 1 {
			baseline = math.Exp(res[1])
			divergence = res[4]
			zscore = res[6]
		}

		for value := range data.NewValue(baseline, divergence, zscore, defined).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
