package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RMS owns sqrt(sum(x²) / n).
*/
type RMS struct {
	err    error
	count  float64
	energy float64
	out    float64
}

func NewRMS() core.Primitive {
	return &RMS{}
}

func (op *RMS) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		count, energy := 0.0, 0.0

		for arriving := range in {
			val := *(*float64)(arriving)
			count++
			energy += val * val
			op.out = math.Sqrt(energy / count)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *RMS) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
