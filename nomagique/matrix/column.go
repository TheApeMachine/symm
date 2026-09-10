package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Column arranges a scalar run as an n-by-one matrix.
*/
type Column struct {
	core.Base[float64, [][]float64]
}

func NewColumn() *Column {
	return &Column{}
}

func (op *Column) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, arriving.Read())
		}

		rows := make([][]float64, len(values))

		for index, value := range values {
			rows[index] = []float64{value}
		}

		if !yield(op.Carrier(rows)) {
			return
		}
	}
}
