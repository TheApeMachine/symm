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
	err error
	out float64
}

func NewPredictiveInflation() core.Primitive {
	return &PredictiveInflation{}
}

func (op *PredictiveInflation) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			count := *(*float64)(arriving)
			op.out = math.Sqrt(1.0 + 1.0/count)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *PredictiveInflation) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
