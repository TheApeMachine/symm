package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
StandardizerResult preserves the source's inclusive score using moments after
incorporation.
*/
type StandardizerResult struct {
	MomentReading
	ZScore float64
}

/*
Standardizer is deliberately distinct from CausalResidual: it scores against
the posterior, not the prior.
*/
type Standardizer struct {
	core.Base[float64, StandardizerResult]
	moments core.Primitive[float64, MomentReading]
}

func NewStandardizer(moments core.Primitive[float64, MomentReading]) *Standardizer {
	return &Standardizer{moments: moments}
}

func (op *Standardizer) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[StandardizerResult, StandardizerResult]] {
	return func(yield func(core.Primitive[StandardizerResult, StandardizerResult]) bool) {
		for reading := range op.moments.Next(in) {
			state := reading.Read()
			result := StandardizerResult{MomentReading: state}

			if state.Dispersion > 0 {
				result.ZScore = (state.Value - state.Mean) / state.Dispersion
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}

		op.Error(op.moments.Error())
	}
}
