package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
OuterInput is two vectors lifted to a column and a row.
*/
type OuterInput struct {
	Left  []float64
	Right []float64
}

/*
Outer owns the outer product left ⊗ right.
*/
type Outer struct {
	*core.PrimitiveError

	out [][]float64
}

func NewOuter() *Outer {
	return &Outer{PrimitiveError: core.NewPrimitiveError()}
}

func (outer *Outer) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*OuterInput)(arriving)
			outer.out = make([][]float64, len(input.Left))
			values := make([]float64, len(input.Left)*len(input.Right))
			width := len(input.Right)

			for row, lval := range input.Left {
				outer.out[row] = values[row*width : (row+1)*width]

				for col, rval := range input.Right {
					outer.out[row][col] = lval * rval
				}
			}

			if !yield(unsafe.Pointer(&outer.out)) {
				return
			}
		}
	}
}
