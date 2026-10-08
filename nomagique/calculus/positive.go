package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Positive restricts a scalar run to the strictly positive domain. It does not
// clamp, replace, or drop an invalid operand. An invalid run yields no values.
type Positive struct{ *core.PrimitiveError }

func NewPositive() core.Primitive {
	return &Positive{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Positive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)

			if !(value > 0) {
				op.Error(core.ErrDomain)
				return
			}

			values = append(values, value)
		}

		for index := range values {
			if !yield(unsafe.Pointer(&values[index])) {
				return
			}
		}
	}
}
