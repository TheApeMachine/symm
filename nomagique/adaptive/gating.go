package adaptive

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Gating suppresses values inside a configured threshold of inclusive moments.
*/
type Gating struct {
	core.Base[float64, float64]
	moments   core.Primitive[float64, equation.MomentReading]
	threshold core.Primitive[float64, float64]
}

func NewGating(
	moments core.Primitive[float64, equation.MomentReading],
	threshold core.Primitive[float64, float64],
) *Gating {
	return &Gating{moments: moments, threshold: threshold}
}

func (op *Gating) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for reading := range op.moments.Next(in) {
			current := reading.Read()
			value := current.Value
			limit, err := transport.Evaluate(op.threshold, transport.Values(current.Count))

			if err != nil {
				op.Error(err)
				return
			}

			if current.Dispersion > 0 && math.Abs(current.Value-current.Mean) < limit {
				value = 0
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}

		op.Error(op.moments.Error())
	}
}
