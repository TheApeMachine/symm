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
	tick   int64
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
		var out [2]float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				continue
			}

			if op.fixed {
				res := *(*float64)(arriving) * op.factor
				if !yield(unsafe.Pointer(&res)) {
					return
				}
				continue
			}

			current := op.tick % 2
			out[current] = *(*float64)(arriving)
			op.tick++

			if current == 1 {
				res := out[0] * out[1]
				if !yield(unsafe.Pointer(&res)) {
					return
				}
			}
		}
	}
}
