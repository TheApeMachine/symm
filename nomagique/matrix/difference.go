package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DifferenceInput is a pair of equally shaped matrices.
*/
type DifferenceInput struct {
	Left  [][]float64
	Right [][]float64
}

/*
Difference subtracts equally shaped matrices in typed coefficient storage.
*/
type Difference struct {
	core.Base[DifferenceInput, [][]float64]
}

func NewDifference() *Difference {
	return &Difference{}
}

func (op *Difference) Next(
	in iter.Seq[core.Primitive[DifferenceInput, DifferenceInput]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		for arriving := range in {
			rows := op.Subtract(arriving.Read().Left, arriving.Read().Right)

			if op.Error() != nil {
				return
			}

			if !yield(op.Carrier(rows)) {
				return
			}
		}
	}
}

/*
Subtract preserves both operands and rejects unequal shapes.
*/
func (op *Difference) Subtract(left, right [][]float64) [][]float64 {
	if len(left) != len(right) {
		op.Error(core.ErrShape)
		return nil
	}

	rows := make([][]float64, len(left))

	for row, values := range left {
		if len(values) != len(right[row]) {
			op.Error(core.ErrShape)
			return nil
		}

		rows[row] = make([]float64, len(values))

		for column, value := range values {
			rows[row][column] = value - right[row][column]
		}
	}

	return rows
}
