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
	return &RMS{PrimitiveError: core.NewPrimitiveError()}
}

func (rms *RMS) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		count, energy := 0.0, 0.0

		for arriving := range in {
			val := *(*float64)(arriving)
			count++
			energy += val * val
			rms.out = math.Sqrt(energy / count)

			if !yield(unsafe.Pointer(&rms.out)) {
				return
			}
		}
	}
}
