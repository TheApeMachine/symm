package matrix

import (
	"iter"

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
	core.Base[OuterInput, [][]float64]
	product Product
}

func NewOuter() *Outer {
	return &Outer{}
}

func (op *Outer) Next(
	in iter.Seq[core.Primitive[OuterInput, OuterInput]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			column := make([][]float64, len(input.Left))

			for index, value := range input.Left {
				column[index] = []float64{value}
			}

			row := [][]float64{append([]float64(nil), input.Right...)}
			product := op.product.Multiply(column, row)

			if op.product.Error() != nil {
				op.Error(op.product.Error())
				return
			}

			if !yield(op.Carrier(product)) {
				return
			}
		}
	}
}
