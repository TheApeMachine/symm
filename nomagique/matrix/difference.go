package matrix

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Difference subtracts equally shaped matrices in typed coefficient storage.
Each arrival is *[2][][]float64 {left, right}; it yields *[][]float64.
*/
type Difference struct {
	*core.PrimitiveError
	out [][]float64
}

func NewDifference() core.Primitive {
	return &Difference{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Difference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*[2][][]float64)(arriving)
			left, right := input[0], input[1]

			if len(left) != len(right) {
				op.Error(core.ErrShape)
				return
			}

			op.out = make([][]float64, len(left))
			ok := true

			for row, values := range left {
				if len(values) != len(right[row]) {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				op.out[row] = make([]float64, len(values))

				for column, value := range values {
					op.out[row][column] = value - right[row][column]
				}
			}

			if !ok {
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
