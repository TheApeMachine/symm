package matrix

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Outer owns the outer product left ⊗ right. Each arrival is *[2][]float64
{left, right}; it yields *[][]float64.
*/
type Outer struct {
	*core.PrimitiveError
	out [][]float64
}

func NewOuter() core.Primitive {
	return &Outer{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Outer) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*[2][]float64)(arriving)
			left, right := input[0], input[1]
			op.out = make([][]float64, len(left))
			values := make([]float64, len(left)*len(right))
			width := len(right)

			for row, lval := range left {
				op.out[row] = values[row*width : (row+1)*width]

				for col, rval := range right {
					op.out[row][col] = lval * rval
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
