package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Diagonal selects row i's member i. An undersized row is a shape error.
*/
type Diagonal struct {
	core.Base[[][]float64, []float64]
}

func NewDiagonal() *Diagonal {
	return &Diagonal{}
}

func (op *Diagonal) Next(
	in iter.Seq[core.Primitive[[][]float64, [][]float64]],
) iter.Seq[core.Primitive[[]float64, []float64]] {
	return func(yield func(core.Primitive[[]float64, []float64]) bool) {
		for arriving := range in {
			rows := arriving.Read()
			diag := make([]float64, len(rows))
			ok := true

			for index, row := range rows {
				if index >= len(row) {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				diag[index] = row[index]
			}

			if !ok {
				continue
			}

			if !yield(op.Carrier(diag)) {
				return
			}
		}
	}
}
