package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PredictiveInflation owns sqrt(1 + 1/count).
*/
type PredictiveInflation struct {
	*core.PrimitiveError

	out float64
}

func NewPredictiveInflation() *PredictiveInflation {
	return &PredictiveInflation{PrimitiveError: core.NewPrimitiveError()}
}

func (predictiveInflation *PredictiveInflation) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			count := *(*float64)(arriving)
			predictiveInflation.out = math.Sqrt(1.0 + 1.0/count)

			if !yield(unsafe.Pointer(&predictiveInflation.out)) {
				return
			}
		}
	}
}
