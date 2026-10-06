package vector

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Scale multiplies each member by one scalar.
*/
type Scale struct {
	*core.PrimitiveError
	factor float64
	fixed  bool
	out    []float64
}

/*
NewScale constructs a Scale primitive. If a factor is provided, arrivals are
*[]float64 and every member is multiplied by it. Otherwise arrivals are
*[2][]float64 {values, {factor}}: the factor travels as the single member of
the second operand. It yields *[]float64.
*/
func NewScale(factor ...float64) core.Primitive {
	op := &Scale{
		PrimitiveError: core.NewPrimitiveError(),
	}

	if len(factor) > 0 {
		op.factor = factor[0]
		op.fixed = true
	}

	return op
}

func (op *Scale) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			factor := op.factor
			var values []float64

			if op.fixed {
				values = *(*[]float64)(arriving)
			} else {
				input := (*[2][]float64)(arriving)

				if len(input[1]) != 1 {
					op.Error(core.ErrShape)
					return
				}

				values, factor = input[0], input[1][0]
			}

			if len(op.out) != len(values) {
				op.out = make([]float64, len(values))
			}

			for index, value := range values {
				op.out[index] = value * factor
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
