package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ProductInput is a pair of matrices.
*/
type ProductInput struct {
	Left  [][]float64
	Right [][]float64
}

/*
Product owns matrix multiplication. Coefficients stay in contiguous float64
storage.
*/
type Product struct {
	*core.PrimitiveError

	out [][]float64
}

func NewProduct() *Product {
	return &Product{PrimitiveError: core.NewPrimitiveError()}
}

func (product *Product) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ProductInput)(arriving)
			width := 0

			if len(input.Right) > 0 {
				width = len(input.Right[0])
			}

			ok := true

			for _, row := range input.Right {
				if len(row) != width {
					product.Error(core.ErrShape)
					ok = false
					break
				}
			}

			if !ok {
				return
			}

			for _, row := range input.Left {
				if len(row) != len(input.Right) {
					product.Error(core.ErrShape)
					ok = false
					break
				}
			}

			if !ok {
				return
			}

			product.out = make([][]float64, len(input.Left))
			values := make([]float64, len(input.Left)*width)

			for row, coefficients := range input.Left {
				product.out[row] = values[row*width : (row+1)*width]

				for inner, coefficient := range coefficients {
					for column, value := range input.Right[inner] {
						product.out[row][column] += coefficient * value
					}
				}
			}

			if !yield(unsafe.Pointer(&product.out)) {
				return
			}
		}
	}
}
