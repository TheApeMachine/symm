package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PredictiveInflation owns sqrt(1 + 1/count).
*/
type PredictiveInflation struct {
	core.Base[float64, float64]
}

func NewPredictiveInflation() *PredictiveInflation {
	return &PredictiveInflation{}
}

func (op *PredictiveInflation) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			count := arriving.Read()

			if !yield(op.Carrier(math.Sqrt(1 + 1/count))) {
				return
			}
		}
	}
}
