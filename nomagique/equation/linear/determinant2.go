package linear

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Matrix2 is the four entries of a 2-by-2 matrix.
*/
type Matrix2 struct {
	A, B, C, D float64
}

/*
Determinant2 owns ad − bc.
*/
type Determinant2 struct {
	core.Base[Matrix2, float64]
}

func NewDeterminant2() *Determinant2 {
	return &Determinant2{}
}

func (op *Determinant2) Next(
	in iter.Seq[core.Primitive[Matrix2, Matrix2]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			m := arriving.Read()

			if !yield(op.Carrier(m.A*m.D - m.B*m.C)) {
				return
			}
		}
	}
}
