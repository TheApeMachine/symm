package matrix

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Determinant2 owns ad − bc. Each arrival is *[4]float64 {a, b, c, d}, the
row-major entries of a 2-by-2 matrix; it yields *float64.
*/
type Determinant2 struct {
	*core.PrimitiveError
	out float64
}

func NewDeterminant2() core.Primitive {
	return &Determinant2{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Determinant2) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := (*[4]float64)(arriving)
			op.out = m[0]*m[3] - m[1]*m[2]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
