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
	return &PredictiveInflation{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *PredictiveInflation) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			count := *(*float64)(arriving)

			if count <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.out = math.Sqrt(1.0 + 1.0/count)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
