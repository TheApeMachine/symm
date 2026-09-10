package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ExtremeScale owns sqrt(2 log n), the coefficient of the Gaussian/EVT envelope
identity. It adds no tail calibration claim.
*/
type ExtremeScale[U core.Floating] struct {
	core.Base[U, U]
}

func NewExtremeScale[U core.Floating]() *ExtremeScale[U] {
	return &ExtremeScale[U]{}
}

func (op *ExtremeScale[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			count := arriving.Read()

			if !yield(op.Carrier(U(math.Sqrt(2 * math.Log(float64(count)))))) {
				return
			}
		}
	}
}
