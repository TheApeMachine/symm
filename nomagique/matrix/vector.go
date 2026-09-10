package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
VectorInput is a matrix and the vector it multiplies.
*/
type VectorInput struct {
	Matrix [][]float64
	Vector []float64
}

/*
Vector multiplies a matrix by a vector and retains the vector result.
*/
type Vector struct {
	core.Base[VectorInput, []float64]
	product Product
}

func NewVector() *Vector {
	return &Vector{}
}

func (op *Vector) Next(
	in iter.Seq[core.Primitive[VectorInput, VectorInput]],
) iter.Seq[core.Primitive[[]float64, []float64]] {
	return func(yield func(core.Primitive[[]float64, []float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			column := make([][]float64, len(input.Vector))

			for index, value := range input.Vector {
				column[index] = []float64{value}
			}

			product := op.product.Multiply(input.Matrix, column)

			if op.product.Error() != nil {
				op.Error(op.product.Error())
				return
			}

			result := make([]float64, len(product))

			for index, row := range product {
				if len(row) != 1 {
					op.Error(core.ErrShape)
					return
				}

				result[index] = row[0]
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}
