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
	*core.PrimitiveError
	count  float64
	energy float64
	out    float64
}

func NewRMS() *RMS {
	return &RMS{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *RMS) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			op.count++
			op.energy += val * val
			op.out = math.Sqrt(op.energy / op.count)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
