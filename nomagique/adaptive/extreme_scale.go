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
	err error
	out float64
}

func NewExtremeScale() core.Primitive {
	return &ExtremeScale{}
}

func (op *ExtremeScale) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			count := *(*float64)(arriving)
			op.out = math.Sqrt(2.0 * math.Log(count))

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *ExtremeScale) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
