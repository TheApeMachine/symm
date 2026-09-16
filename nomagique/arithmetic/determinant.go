package arithmetic

import (
	"iter"
	"unsafe"

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
	*core.PrimitiveError

	out float64
}

func NewDeterminant2() *Determinant2 {
	return &Determinant2{PrimitiveError: core.NewPrimitiveError()}
}

func (determinant2 *Determinant2) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*Matrix2)(arriving)
			determinant2.out = m.A*m.D - m.B*m.C

			if !yield(unsafe.Pointer(&determinant2.out)) {
				return
			}
		}
	}
}
