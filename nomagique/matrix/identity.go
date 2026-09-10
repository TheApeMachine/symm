package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Identity constructs I_n from an arriving dimension. The diagonal is written in
typed storage; there is no scalar graph per coefficient.
*/
type Identity struct {
	core.Base[float64, [][]float64]
}

func NewIdentity() *Identity {
	return &Identity{}
}

func (op *Identity) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		for arriving := range in {
			size := int(arriving.Read())

			if float64(size) != arriving.Read() || size < 0 {
				op.Error(core.ErrShape)
				return
			}

			rows := make([][]float64, size)
			values := make([]float64, size*size)

			for row := range rows {
				rows[row] = values[row*size : (row+1)*size]
				rows[row][row] = 1
			}

			if !yield(op.Carrier(rows)) {
				return
			}
		}
	}
}
