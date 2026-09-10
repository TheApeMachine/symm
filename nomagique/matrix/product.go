package matrix

import (
	"iter"

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
	core.Base[ProductInput, [][]float64]
}

func NewProduct() *Product {
	return &Product{}
}

func (op *Product) Next(
	in iter.Seq[core.Primitive[ProductInput, ProductInput]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		for arriving := range in {
			product := op.Multiply(arriving.Read().Left, arriving.Read().Right)

			if op.Error() != nil {
				return
			}

			if !yield(op.Carrier(product)) {
				return
			}
		}
	}
}

/*
Multiply returns a fresh rectangular product without mutating either operand.
*/
func (op *Product) Multiply(left, right [][]float64) [][]float64 {
	width := 0

	if len(right) > 0 {
		width = len(right[0])
	}

	for _, row := range right {
		if len(row) != width {
			op.Error(core.ErrShape)
			return nil
		}
	}

	for _, row := range left {
		if len(row) != len(right) {
			op.Error(core.ErrShape)
			return nil
		}
	}

	rows := make([][]float64, len(left))
	values := make([]float64, len(left)*width)

	for row, coefficients := range left {
		rows[row] = values[row*width : (row+1)*width]

		for inner, coefficient := range coefficients {
			for column, value := range right[inner] {
				rows[row][column] += coefficient * value
			}
		}
	}

	return rows
}
