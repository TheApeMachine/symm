package matrix

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Product owns matrix multiplication. Each arrival is *[2][][]float64
{left, right}; it yields *[][]float64. Coefficients stay in contiguous float64
storage.
*/
type Product struct {
	*core.PrimitiveError
	out [][]float64
}

func NewProduct() core.Primitive {
	return &Product{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Product) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*[2][][]float64)(arriving)
			left, right := input[0], input[1]
			width := 0

			if len(right) > 0 {
				width = len(right[0])
			}

			ok := true

			for _, row := range right {
				if len(row) != width {
					op.Error(core.ErrShape)
					ok = false
					break
				}
			}

			if !ok {
				return
			}

			for _, row := range left {
				if len(row) != len(right) {
					op.Error(core.ErrShape)
					ok = false
					break
				}
			}

			if !ok {
				return
			}

			op.out = make([][]float64, len(left))
			values := make([]float64, len(left)*width)

			for row, coefficients := range left {
				op.out[row] = values[row*width : (row+1)*width]

				for inner, coefficient := range coefficients {
					for column, value := range right[inner] {
						op.out[row][column] += coefficient * value
					}
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
