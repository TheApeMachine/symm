package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ExtremeScale owns sqrt(2 log n), the coefficient of the Gaussian/EVT envelope
identity.
*/
type ExtremeScale struct {
	*core.PrimitiveError

	out float64
}

func NewExtremeScale() *ExtremeScale {
	return &ExtremeScale{PrimitiveError: core.NewPrimitiveError()}
}

func (extremeScale *ExtremeScale) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			count := *(*float64)(arriving)
			extremeScale.out = math.Sqrt(2.0 * math.Log(count))

			if !yield(unsafe.Pointer(&extremeScale.out)) {
				return
			}
		}
	}
}
