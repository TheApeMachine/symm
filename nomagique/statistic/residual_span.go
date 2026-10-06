package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ResidualSpan reproduces the supplied calibration range update.

Each arrival is the running calibration range and the arriving residual as
*[4]float64 {count, minimum, maximum, residual}. It yields the source's state
counter and the observed residual range as *[4]float64
{count, minimum, maximum, span}.
*/
type ResidualSpan struct {
	*core.PrimitiveError
	out [4]float64
}

func NewResidualSpan() *ResidualSpan {
	return &ResidualSpan{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *ResidualSpan) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[4]float64)(arriving)
			count, minimum, maximum, residual := input[0], input[1], input[2], input[3]
			outCount, outMinimum, outMaximum := count, minimum, maximum

			if count == 0 {
				outMinimum = residual
				outMaximum = residual
				outCount = 1
			}

			if outCount > 1 {
				if residual < outMinimum {
					outMinimum = residual
				}

				if residual > outMaximum {
					outMaximum = residual
				}

				outCount = count + 1
			}

			if outCount == 1 && residual != outMinimum {
				if residual < outMinimum {
					outMinimum = residual
				}

				if residual > outMaximum {
					outMaximum = residual
				}

				outCount = 2
			}

			op.out = [4]float64{outCount, outMinimum, outMaximum, outMaximum - outMinimum}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
